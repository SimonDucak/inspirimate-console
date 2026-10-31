package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPatchAppNotifyEmails(t *testing.T) {
	s := appsGitServer(t)
	w := createApp(s, `{"name":"notes"}`, "tenantA")
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created v1App
	if json.Unmarshal(w.Body.Bytes(), &created) != nil || created.ID == "" {
		t.Fatal(w.Body.String())
	}
	if len(created.NotifyEmails) != 0 {
		t.Fatalf("new app emails: %#v", created.NotifyEmails)
	}
	w = patchApp(s, created.ID, `{"notify_emails":[" Person@Example.com ","person@example.com","bad"]}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid email accepted: %d %s", w.Code, w.Body.String())
	}
	w = patchApp(s, created.ID, `{"notify_emails":[" Person@Example.com ","person@example.com"]}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"notify_emails":["person@example.com"]`) {
		t.Fatalf("save %d %s", w.Code, w.Body.String())
	}
	w = patchApp(s, created.ID, `{"notify_emails":[]}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"notify_emails":[]`) {
		t.Fatalf("clear %d %s", w.Code, w.Body.String())
	}
}

func patchApp(s *Server, id, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := reqAs("PATCH", "/v1/apps/"+id, body, "tenantA")
	r.SetPathValue("id", id)
	s.v1PatchApp(w, r)
	return w
}
