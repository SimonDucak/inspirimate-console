package supabaseproxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRedactAuthDropsSecrets(t *testing.T) {
	raw := []byte(`{"external_google_enabled":true,"external_google_client_id":"id","external_google_secret":"hide-me","jwt_secret":"also"}`)
	out, err := redactAuth(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "hide-me") || strings.Contains(string(out), "also") {
		t.Fatalf("secret leaked: %s", out)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["external_google_enabled"] != true || doc["external_google_client_id"] != "id" {
		t.Fatalf("kept fields = %s", out)
	}
}

func TestPickClientKeyDropsServiceRole(t *testing.T) {
	got := pickClientKey([]projectAPIKey{
		{Name: "service_role", Type: "legacy", APIKey: "service-secret"},
		{Name: "default", Type: "secret", APIKey: "sb_secret_hide"},
		{Name: "anon", Type: "legacy", APIKey: "anon-public"},
		{Name: "default", Type: "publishable", APIKey: "sb_publishable_use"},
	})
	if got != "sb_publishable_use" {
		t.Fatalf("key = %q", got)
	}
}

func TestPublicClientReadsPublishableKey(t *testing.T) {
	const token = "sbp_test_token"
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("auth header missing")
		}
		if strings.Contains(r.URL.RawQuery, "reveal") == false {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `[{"name":"service_role","api_key":"service-secret"},{"name":"anon","api_key":"anon-public"}]`)
	}))
	defer up.Close()
	prev := projectAPIKeysURL
	projectAPIKeysURL = up.URL + "/%s/api-keys?reveal=true"
	t.Cleanup(func() { projectAPIKeysURL = prev })

	apiURL, key, err := PublicClient(t.Context(), token, "abcd1234")
	if err != nil {
		t.Fatal(err)
	}
	if apiURL != "https://abcd1234.supabase.co" || key != "anon-public" {
		t.Fatalf("url %s key %s", apiURL, key)
	}
}

func TestAuthToolsDescribeSignIn(t *testing.T) {
	tools := authTools()
	if len(tools) != 3 || tools[0]["name"] != "get_auth_config" || tools[1]["name"] != "enable_google_sign_in" || tools[2]["name"] != "update_auth_config" {
		t.Fatalf("tools = %+v", tools)
	}
}

func TestGoogleSignInRequiresBothSecrets(t *testing.T) {
	if changes, _ := googleSignInChanges("", "secret"); changes != nil {
		t.Fatal("missing client id was accepted")
	}
	changes, msg := googleSignInChanges("google-client", "google-secret")
	if msg != "" || changes["external_google_enabled"] != true || changes["external_google_client_id"] != "google-client" || changes["external_google_secret"] != "google-secret" {
		t.Fatalf("changes = %#v msg = %q", changes, msg)
	}
}

func TestProxyInjectsTokenAndLocksFeatures(t *testing.T) {
	const token = "sbp_test_token"
	var gotAuth, gotFeatures, gotRef string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotFeatures = r.URL.Query().Get("features")
		gotRef = r.URL.Query().Get("project_ref")
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer up.Close()
	prev := mcpEndpoint
	mcpEndpoint = up.URL
	t.Cleanup(func() { mcpEndpoint = prev })

	reg := NewRegistry()
	key, err := reg.Put(Grant{
		Token: token, ProjectRef: "abcd1234", Features: []string{"database", "storage"},
		Auth: true, Expires: time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	h := reg.Handler()

	req := httptest.NewRequest(http.MethodPost, "/supabase/"+key+"/mcp?features=account,database", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mcp status %d body %s", rec.Code, rec.Body.String())
	}
	if gotAuth != "Bearer "+token {
		t.Fatal("upstream did not receive the connection token")
	}
	if gotFeatures != "database,storage" || gotRef != "abcd1234" {
		t.Fatalf("upstream query features=%q ref=%q", gotFeatures, gotRef)
	}
	if strings.Contains(rec.Body.String(), token) {
		t.Fatal("token leaked to the agent")
	}

	miss := httptest.NewRecorder()
	h.ServeHTTP(miss, httptest.NewRequest(http.MethodPost, "/supabase/not-a-key/mcp", nil))
	if miss.Code != http.StatusNotFound || strings.Contains(miss.Body.String(), token) {
		t.Fatalf("unknown key status %d body %s", miss.Code, miss.Body.String())
	}

	initReq := httptest.NewRequest(http.MethodPost, "/supabase/"+key+"/manage", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	initRec := httptest.NewRecorder()
	h.ServeHTTP(initRec, initReq)
	if initRec.Code != http.StatusOK || strings.Contains(initRec.Body.String(), token) {
		t.Fatalf("manage status %d body %s", initRec.Code, initRec.Body.String())
	}
	var rpc struct {
		Result struct {
			ServerInfo struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.Unmarshal(initRec.Body.Bytes(), &rpc); err != nil || rpc.Result.ServerInfo.Name != "supabase-auth" {
		t.Fatalf("manage initialize: %s", initRec.Body.String())
	}
}
