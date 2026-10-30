// v1_supabase_connections.go — owner-scoped Supabase connections. The personal
// access token is write-only: sealed with secrets.Cipher, never returned, and
// never logged.
package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tastyeffectco/sandboxd/control-plane/internal/audit"
	"github.com/tastyeffectco/sandboxd/control-plane/internal/auth"
	"github.com/tastyeffectco/sandboxd/control-plane/internal/store"
)

var supabaseProjectRef = regexp.MustCompile(`^[a-z0-9]{8,40}$`)

type v1SupabaseConnection struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	ProjectRef string   `json:"project_ref"`
	Features   []string `json:"features"`
	Management []string `json:"management"`
	TokenSet   bool     `json:"token_set"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at,omitempty"`
}

func toV1SupabaseConnection(c *store.SupabaseConnection) v1SupabaseConnection {
	return v1SupabaseConnection{
		ID:         c.ID,
		Name:       c.Name,
		ProjectRef: c.ProjectRef,
		Features:   decodeStringList(c.Features),
		Management: decodeStringList(c.Management),
		TokenSet:   true,
		CreatedAt:  c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  c.UpdatedAt.Format(time.RFC3339),
	}
}

func decodeStringList(raw string) []string {
	var out []string
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		return []string{}
	}
	return out
}

// POST /v1/supabase-connections — store an encrypted token. Returns metadata only.
func (s *Server) v1CreateSupabaseConnection(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil || s.Secrets == nil {
		writeV1Err(w, http.StatusServiceUnavailable, "unavailable", "supabase connection store not configured")
		return
	}
	var body struct {
		Name       string   `json:"name"`
		ProjectRef string   `json:"project_ref"`
		Features   []string `json:"features"`
		Management []string `json:"management"`
		Token      string   `json:"token"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	name := strings.TrimSpace(body.Name)
	ref := strings.TrimSpace(body.ProjectRef)
	features, management, scopeOK := normalizeSupabaseScope(body.Features, body.Management)
	switch {
	case !validLabel(name, 100):
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "name must be 1-100 printable characters")
		return
	case !supabaseProjectRef.MatchString(ref):
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "project_ref must be 8-40 lowercase letters or digits")
		return
	case !scopeOK:
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "choose database, docs, storage, auth, or a combination")
		return
	case !validToken(body.Token):
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "token must be 1-4096 chars with no NUL/CR/LF")
		return
	}
	ct, nonce, err := s.Secrets.Seal([]byte(body.Token))
	if err != nil {
		writeV1Err(w, http.StatusInternalServerError, "internal", "could not encrypt token")
		return
	}
	c := &store.SupabaseConnection{
		ID:         newULID(),
		OwnerToken: tenantToken(r),
		Name:       name,
		ProjectRef: ref,
		Features:   features,
		Management: management,
	}
	if err := s.Store.CreateSupabaseConnection(r.Context(), c, ct, nonce); err != nil {
		if err == store.ErrConflict {
			writeV1Err(w, http.StatusConflict, "conflict", "a connection with that name already exists")
			return
		}
		writeV1Err(w, http.StatusInternalServerError, "internal", "could not store connection")
		return
	}
	s.auditSupabase(r, "supabase_connection.created", c.ID, c.Name)
	writeJSON(w, http.StatusCreated, toV1SupabaseConnection(c))
}

// GET /v1/supabase-connections — owner-scoped list, metadata only (no token).
func (s *Server) v1ListSupabaseConnections(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil {
		writeV1Err(w, http.StatusServiceUnavailable, "unavailable", "supabase connection store not configured")
		return
	}
	rows, err := s.Store.ListSupabaseConnections(r.Context(), tenantToken(r))
	if err != nil {
		writeV1Err(w, http.StatusInternalServerError, "internal", "could not list connections")
		return
	}
	out := make([]v1SupabaseConnection, 0, len(rows))
	for _, c := range rows {
		out = append(out, toV1SupabaseConnection(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": out})
}

// PATCH /v1/supabase-connections/{id} — update metadata. A blank token keeps
// the saved token. A non-blank token replaces it. The token is never returned.
func (s *Server) v1UpdateSupabaseConnection(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil || s.Secrets == nil {
		writeV1Err(w, http.StatusServiceUnavailable, "unavailable", "supabase connection store not configured")
		return
	}
	var body struct {
		Name       string   `json:"name"`
		ProjectRef string   `json:"project_ref"`
		Features   []string `json:"features"`
		Management []string `json:"management"`
		Token      string   `json:"token"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	name := strings.TrimSpace(body.Name)
	ref := strings.TrimSpace(body.ProjectRef)
	features, management, scopeOK := normalizeSupabaseScope(body.Features, body.Management)
	token := body.Token
	switch {
	case !validLabel(name, 100):
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "name must be 1-100 printable characters")
		return
	case !supabaseProjectRef.MatchString(ref):
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "project_ref must be 8-40 lowercase letters or digits")
		return
	case !scopeOK:
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "choose database, docs, storage, auth, or a combination")
		return
	case token != "" && !validToken(token):
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "token must be 1-4096 chars with no NUL/CR/LF")
		return
	}
	var ct, nonce []byte
	if token != "" {
		var err error
		ct, nonce, err = s.Secrets.Seal([]byte(token))
		if err != nil {
			writeV1Err(w, http.StatusInternalServerError, "internal", "could not encrypt token")
			return
		}
	}
	c, found, err := s.Store.UpdateSupabaseConnection(r.Context(), tenantToken(r), r.PathValue("id"), name, ref, features, management, ct, nonce, token != "")
	if err != nil {
		if err == store.ErrConflict {
			writeV1Err(w, http.StatusConflict, "conflict", "a connection with that name already exists")
			return
		}
		writeV1Err(w, http.StatusInternalServerError, "internal", "could not update connection")
		return
	}
	if !found {
		writeV1Err(w, http.StatusNotFound, "not_found", "no such connection")
		return
	}
	s.auditSupabase(r, "supabase_connection.updated", c.ID, c.Name)
	writeJSON(w, http.StatusOK, toV1SupabaseConnection(c))
}

// DELETE /v1/supabase-connections/{id} — owner-scoped delete.
func (s *Server) v1DeleteSupabaseConnection(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil {
		writeV1Err(w, http.StatusServiceUnavailable, "unavailable", "supabase connection store not configured")
		return
	}
	id := r.PathValue("id")
	deleted, err := s.Store.DeleteSupabaseConnection(r.Context(), tenantToken(r), id)
	if err != nil {
		writeV1Err(w, http.StatusInternalServerError, "internal", "could not delete connection")
		return
	}
	if !deleted {
		writeV1Err(w, http.StatusNotFound, "not_found", "no such connection")
		return
	}
	s.auditSupabase(r, "supabase_connection.deleted", id, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) auditSupabase(r *http.Request, action, id, name string) {
	if s.Audit == nil {
		return
	}
	actor := auth.ActorFrom(r.Context())
	detail := map[string]any{}
	if name != "" {
		detail["name"] = name
	}
	s.Audit.Write(r.Context(), audit.Entry{
		ActorKind: actor.Kind,
		ActorName: actor.Name,
		Action:    action,
		Target:    id,
		Detail:    detail,
	})
}

// normalizeSupabaseScope checks MCP feature groups and Management API areas.
// features may be database, docs, and storage. management may be auth and
// storage. Either list may be empty. At least one value must be chosen.
// Both stored forms are sorted JSON arrays.
func normalizeSupabaseScope(features, management []string) (string, string, bool) {
	featureJSON, featuresOK := normalizeChoice(features, map[string]bool{"database": true, "docs": true, "storage": true})
	managementJSON, managementOK := normalizeChoice(management, map[string]bool{"auth": true, "storage": true})
	if !featuresOK || !managementOK || (featureJSON == "[]" && managementJSON == "[]") {
		return "", "", false
	}
	return featureJSON, managementJSON, true
}

func normalizeChoice(in []string, allowed map[string]bool) (string, bool) {
	seen := map[string]bool{}
	var out []string
	for _, f := range in {
		f = strings.TrimSpace(f)
		if !allowed[f] {
			return "", false
		}
		if seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	if out == nil {
		out = []string{}
	}
	sort.Strings(out)
	b, err := json.Marshal(out)
	if err != nil {
		return "", false
	}
	return string(b), true
}
