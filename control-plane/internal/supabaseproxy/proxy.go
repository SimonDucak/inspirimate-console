// Package supabaseproxy lets a sandbox agent use a linked Supabase project
// without the personal access token ever entering the sandbox. The agent calls
// a task-scoped URL on the control plane. This proxy adds the real token and
// forwards to Supabase.
package supabaseproxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Grant is one task's permission to call Supabase. Token is the personal
// access token and never leaves this process.
type Grant struct {
	Token      string
	ProjectRef string
	Features   []string // MCP groups: database, docs, storage
	Auth       bool
	Expires    time.Time
}

// Registry holds live task grants.
type Registry struct {
	mu     sync.Mutex
	grants map[string]Grant
}

func NewRegistry() *Registry {
	return &Registry{grants: map[string]Grant{}}
}

// Put stores a grant and returns the unguessable path key.
func (r *Registry) Put(g Grant) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	key := hex.EncodeToString(buf)
	r.mu.Lock()
	r.sweepLocked()
	r.grants[key] = g
	r.mu.Unlock()
	return key, nil
}

func (r *Registry) get(key string) (Grant, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sweepLocked()
	g, ok := r.grants[key]
	if !ok || time.Now().After(g.Expires) {
		delete(r.grants, key)
		return Grant{}, false
	}
	return g, true
}

func (r *Registry) sweepLocked() {
	now := time.Now()
	for k, g := range r.grants {
		if now.After(g.Expires) {
			delete(r.grants, k)
		}
	}
}

// mcpEndpoint is the hosted Supabase MCP server. Tests point it at a local server.
var mcpEndpoint = "https://mcp.supabase.com/mcp"

// Handler serves /supabase/{key}/mcp and /supabase/{key}/manage.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(r.serve)
}

func (r *Registry) serve(w http.ResponseWriter, req *http.Request) {
	rest := strings.TrimPrefix(req.URL.Path, "/supabase/")
	key, action, _ := strings.Cut(rest, "/")
	g, ok := r.get(key)
	if !ok {
		http.Error(w, "supabase connection is not active for this task", http.StatusNotFound)
		return
	}
	switch {
	case action == "mcp" || strings.HasPrefix(action, "mcp/"):
		if len(g.Features) == 0 {
			http.Error(w, "this connection has no Supabase MCP tools", http.StatusNotFound)
			return
		}
		r.proxyMCP(w, req, g, strings.TrimPrefix(strings.TrimPrefix(action, "mcp"), "/"))
	case action == "manage" || strings.HasPrefix(action, "manage/"):
		if !g.Auth {
			http.Error(w, "this connection cannot change authentication", http.StatusForbidden)
			return
		}
		r.manage(w, req, g)
	default:
		http.NotFound(w, req)
	}
}

func (r *Registry) proxyMCP(w http.ResponseWriter, req *http.Request, g Grant, suffix string) {
	target, err := url.Parse(mcpEndpoint)
	if err != nil {
		http.Error(w, "supabase mcp unavailable", http.StatusBadGateway)
		return
	}
	if suffix != "" {
		target.Path = strings.TrimRight(target.Path, "/") + "/" + suffix
	}
	q := url.Values{}
	q.Set("project_ref", g.ProjectRef)
	q.Set("features", strings.Join(g.Features, ","))
	target.RawQuery = q.Encode()
	rp := &httputil.ReverseProxy{
		Director: func(out *http.Request) {
			out.URL.Scheme = target.Scheme
			out.URL.Host = target.Host
			out.URL.Path = target.Path
			out.URL.RawQuery = target.RawQuery
			out.Host = target.Host
			out.RequestURI = ""
			out.Header.Set("Authorization", "Bearer "+g.Token)
		},
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "supabase mcp unavailable", http.StatusBadGateway)
		},
	}
	rp.ServeHTTP(w, req)
}

func (r *Registry) manage(w http.ResponseWriter, req *http.Request, g Grant) {
	if req.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var call rpcCall
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20)).Decode(&call); err != nil {
		writeRPCError(w, nil, -32700, "invalid json")
		return
	}
	if call.ID == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	switch call.Method {
	case "initialize":
		writeRPC(w, call.ID, map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "supabase-auth", "version": "1"},
		})
	case "tools/list":
		writeRPC(w, call.ID, map[string]any{"tools": authTools()})
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		_ = json.Unmarshal(call.Params, &params)
		text, isErr := callAuthTool(req, g, params.Name, params.Arguments)
		writeRPC(w, call.ID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"isError": isErr,
		})
	default:
		writeRPCError(w, call.ID, -32601, "method not found")
	}
}

type rpcCall struct {
	ID     any             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func writeRPC(w http.ResponseWriter, id any, result any) {
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func writeRPCError(w http.ResponseWriter, id any, code int, message string) {
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message},
	})
}

func authTools() []map[string]any {
	return []map[string]any{
		{
			"name":        "get_auth_config",
			"description": "Read this Supabase project's sign-in settings. Secrets are removed from the result.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        "enable_google_sign_in",
			"description": "Turn on Google sign-in for this Supabase project. Requires the Google OAuth client id and client secret. Do not call this until the user has pasted both values.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"client_id":     map[string]any{"type": "string"},
					"client_secret": map[string]any{"type": "string"},
				},
				"required": []string{"client_id", "client_secret"},
			},
		},
		{
			"name":        "update_auth_config",
			"description": "Change this project's sign-in settings. Send only the fields that should change. Allowed fields: site_url, uri_allow_list, disable_signup, external_email_enabled, external_anonymous_users_enabled, external_phone_enabled, mailer_autoconfirm, external_google_enabled, external_google_client_id, external_google_secret, external_apple_enabled, external_apple_client_id, external_apple_secret.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"changes": map[string]any{"type": "object"},
				},
				"required": []string{"changes"},
			},
		},
	}
}

func googleSignInChanges(clientID, clientSecret string) (map[string]any, string) {
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	if clientID == "" || clientSecret == "" {
		return nil, "Google sign-in needs both a client id and a client secret from the user"
	}
	return map[string]any{
		"external_google_enabled":   true,
		"external_google_client_id": clientID,
		"external_google_secret":    clientSecret,
	}, ""
}

var allowedAuthFields = map[string]bool{
	"site_url": true, "uri_allow_list": true, "disable_signup": true,
	"external_email_enabled": true, "external_anonymous_users_enabled": true,
	"external_phone_enabled": true, "mailer_autoconfirm": true,
	"external_google_enabled": true, "external_google_client_id": true, "external_google_secret": true,
	"external_apple_enabled": true, "external_apple_client_id": true, "external_apple_secret": true,
}

func callAuthTool(req *http.Request, g Grant, name string, args json.RawMessage) (string, bool) {
	switch name {
	case "get_auth_config":
		body, err := supabaseAuth(req, http.MethodGet, g, nil)
		if err != nil {
			return err.Error(), true
		}
		redacted, err := redactAuth(body)
		if err != nil {
			return "could not read auth config", true
		}
		return string(redacted), false
	case "enable_google_sign_in":
		var in struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		}
		_ = json.Unmarshal(args, &in)
		changes, msg := googleSignInChanges(in.ClientID, in.ClientSecret)
		if changes == nil {
			return msg, true
		}
		payload, _ := json.Marshal(changes)
		body, err := supabaseAuth(req, http.MethodPatch, g, payload)
		if err != nil {
			return err.Error(), true
		}
		if _, err := redactAuth(body); err != nil {
			return "Google sign-in is enabled", false
		}
		return "Google sign-in is enabled. Do not repeat the client id or client secret.", false
	case "update_auth_config":
		var in struct {
			Changes map[string]any `json:"changes"`
		}
		if err := json.Unmarshal(args, &in); err != nil || len(in.Changes) == 0 {
			return "changes must be an object with at least one allowed field", true
		}
		clean := map[string]any{}
		for k, v := range in.Changes {
			if allowedAuthFields[k] {
				clean[k] = v
			}
		}
		if len(clean) == 0 {
			return "none of the supplied fields can be changed", true
		}
		payload, _ := json.Marshal(clean)
		body, err := supabaseAuth(req, http.MethodPatch, g, payload)
		if err != nil {
			return err.Error(), true
		}
		redacted, err := redactAuth(body)
		if err != nil {
			return "auth settings were updated", false
		}
		return string(redacted), false
	default:
		return "unknown tool", true
	}
}

func supabaseAuth(req *http.Request, method string, g Grant, payload []byte) ([]byte, error) {
	u := "https://api.supabase.com/v1/projects/" + url.PathEscape(g.ProjectRef) + "/config/auth"
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	out, err := http.NewRequestWithContext(req.Context(), method, u, body)
	if err != nil {
		return nil, err
	}
	out.Header.Set("Authorization", "Bearer "+g.Token)
	if payload != nil {
		out.Header.Set("content-type", "application/json")
	}
	resp, err := http.DefaultClient.Do(out)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, &authError{Status: resp.StatusCode}
	}
	return raw, nil
}

type authError struct{ Status int }

func (e *authError) Error() string {
	return "supabase rejected the auth change (" + http.StatusText(e.Status) + ")"
}

// projectAPIKeysURL is the Management API list of project keys. Tests replace it.
var projectAPIKeysURL = "https://api.supabase.com/v1/projects/%s/api-keys?reveal=true"

// PublicClient returns the project's public API URL and a client key safe to
// embed in an application. Service-role and secret keys are discarded.
func PublicClient(ctx context.Context, token, projectRef string) (apiURL, anonKey string, err error) {
	apiURL = "https://" + projectRef + ".supabase.co"
	if token == "" || projectRef == "" {
		return apiURL, "", fmt.Errorf("missing supabase project")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(projectAPIKeysURL, url.PathEscape(projectRef)), nil)
	if err != nil {
		return apiURL, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return apiURL, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return apiURL, "", &authError{Status: resp.StatusCode}
	}
	var keys []projectAPIKey
	if err := json.Unmarshal(raw, &keys); err != nil {
		return apiURL, "", err
	}
	return apiURL, pickClientKey(keys), nil
}

type projectAPIKey struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	APIKey string `json:"api_key"`
}

func pickClientKey(keys []projectAPIKey) string {
	anon := ""
	for _, k := range keys {
		blob := strings.ToLower(k.Name + " " + k.Type + " " + k.APIKey)
		if k.APIKey == "" || strings.Contains(blob, "service") || strings.Contains(blob, "secret") || strings.HasPrefix(k.APIKey, "sb_secret") {
			continue
		}
		if strings.Contains(blob, "publishable") || strings.HasPrefix(k.APIKey, "sb_publishable") {
			return k.APIKey
		}
		if strings.Contains(blob, "anon") {
			anon = k.APIKey
		}
	}
	return anon
}

// redactAuth removes secret-shaped fields before a tool result reaches the agent.
func redactAuth(raw []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	for k := range doc {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "secret") || strings.Contains(lk, "password") || strings.Contains(lk, "jwt") {
			delete(doc, k)
		}
	}
	return json.Marshal(doc)
}
