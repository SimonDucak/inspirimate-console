// v1_firebase_distributions.go — owner-scoped Firebase App Distribution
// connections. The service account JSON is write-only: sealed with
// secrets.Cipher, never returned, and never logged.
package api

import (
	"encoding/json"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/tastyeffectco/sandboxd/control-plane/internal/audit"
	"github.com/tastyeffectco/sandboxd/control-plane/internal/auth"
	"github.com/tastyeffectco/sandboxd/control-plane/internal/store"
)

const maxFirebaseAccountBytes = 64 * 1024

var firebaseAppIDPattern = regexp.MustCompile(`^1:\d+:android:[0-9a-fA-F]+$`)
var firebaseGroupPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,62}$`)

type v1FirebaseDistribution struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	AppID        string   `json:"app_id"`
	ProjectID    string   `json:"project_id"`
	ClientEmail  string   `json:"client_email"`
	GroupAlias   string   `json:"group_alias"`
	TesterEmails []string `json:"tester_emails"`
	SecretSet    bool     `json:"secret_set"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at,omitempty"`
}

func toV1FirebaseDistribution(c *store.FirebaseDistribution) v1FirebaseDistribution {
	return v1FirebaseDistribution{
		ID:           c.ID,
		Name:         c.Name,
		AppID:        c.AppID,
		ProjectID:    c.ProjectID,
		ClientEmail:  c.ClientEmail,
		GroupAlias:   c.GroupAlias,
		TesterEmails: decodeStringList(c.TesterEmails),
		SecretSet:    true,
		CreatedAt:    c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    c.UpdatedAt.Format(time.RFC3339),
	}
}

type firebaseAccountLabels struct {
	ProjectID   string
	ClientEmail string
}

// POST /v1/firebase-distributions — store an encrypted service account.
func (s *Server) v1CreateFirebaseDistribution(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil || s.Secrets == nil {
		writeV1Err(w, http.StatusServiceUnavailable, "unavailable", "firebase distribution store not configured")
		return
	}
	var body struct {
		Name           string   `json:"name"`
		AppID          string   `json:"app_id"`
		GroupAlias     string   `json:"group_alias"`
		TesterEmails   []string `json:"tester_emails"`
		ServiceAccount string   `json:"service_account"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxFirebaseAccountBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	name := strings.TrimSpace(body.Name)
	appID := strings.TrimSpace(body.AppID)
	group := strings.TrimSpace(body.GroupAlias)
	emails, emailsOK := normalizeFirebaseEmails(body.TesterEmails)
	labels, accountOK := readFirebaseAccount(body.ServiceAccount)
	switch {
	case !validLabel(name, 100):
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "name must be 1-100 printable characters")
		return
	case !firebaseAppIDPattern.MatchString(appID):
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "app_id must look like 1:1234567890:android:abcdef")
		return
	case group != "" && !firebaseGroupPattern.MatchString(group):
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "group_alias must be letters, digits, hyphens, or underscores")
		return
	case !emailsOK:
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "tester emails must be valid addresses")
		return
	case group == "" && len(emails) == 0:
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "add a tester group or at least one tester email")
		return
	case !accountOK:
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "service_account must be a JSON key with project_id, client_email, and private_key")
		return
	}
	ct, nonce, err := s.Secrets.Seal([]byte(body.ServiceAccount))
	if err != nil {
		writeV1Err(w, http.StatusInternalServerError, "internal", "could not encrypt service account")
		return
	}
	rawEmails, _ := json.Marshal(emails)
	c := &store.FirebaseDistribution{
		ID:           newULID(),
		OwnerToken:   tenantToken(r),
		Name:         name,
		AppID:        appID,
		ProjectID:    labels.ProjectID,
		ClientEmail:  labels.ClientEmail,
		GroupAlias:   group,
		TesterEmails: string(rawEmails),
	}
	if err := s.Store.CreateFirebaseDistribution(r.Context(), c, ct, nonce); err != nil {
		if err == store.ErrConflict {
			writeV1Err(w, http.StatusConflict, "conflict", "a connection with that name already exists")
			return
		}
		writeV1Err(w, http.StatusInternalServerError, "internal", "could not store connection")
		return
	}
	s.auditFirebase(r, "firebase_distribution.created", c.ID, c.Name)
	writeJSON(w, http.StatusCreated, toV1FirebaseDistribution(c))
}

// GET /v1/firebase-distributions — metadata only.
func (s *Server) v1ListFirebaseDistributions(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil {
		writeV1Err(w, http.StatusServiceUnavailable, "unavailable", "firebase distribution store not configured")
		return
	}
	rows, err := s.Store.ListFirebaseDistributions(r.Context(), tenantToken(r))
	if err != nil {
		writeV1Err(w, http.StatusInternalServerError, "internal", "could not list connections")
		return
	}
	out := make([]v1FirebaseDistribution, 0, len(rows))
	for _, c := range rows {
		out = append(out, toV1FirebaseDistribution(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": out})
}

// PATCH /v1/firebase-distributions/{id}. A blank service account keeps the saved key.
func (s *Server) v1UpdateFirebaseDistribution(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil || s.Secrets == nil {
		writeV1Err(w, http.StatusServiceUnavailable, "unavailable", "firebase distribution store not configured")
		return
	}
	var body struct {
		Name           string   `json:"name"`
		AppID          string   `json:"app_id"`
		GroupAlias     string   `json:"group_alias"`
		TesterEmails   []string `json:"tester_emails"`
		ServiceAccount string   `json:"service_account"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxFirebaseAccountBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	name := strings.TrimSpace(body.Name)
	appID := strings.TrimSpace(body.AppID)
	group := strings.TrimSpace(body.GroupAlias)
	emails, emailsOK := normalizeFirebaseEmails(body.TesterEmails)
	var labels firebaseAccountLabels
	accountOK := true
	if strings.TrimSpace(body.ServiceAccount) != "" {
		labels, accountOK = readFirebaseAccount(body.ServiceAccount)
	}
	switch {
	case !validLabel(name, 100):
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "name must be 1-100 printable characters")
		return
	case !firebaseAppIDPattern.MatchString(appID):
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "app_id must look like 1:1234567890:android:abcdef")
		return
	case group != "" && !firebaseGroupPattern.MatchString(group):
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "group_alias must be letters, digits, hyphens, or underscores")
		return
	case !emailsOK:
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "tester emails must be valid addresses")
		return
	case group == "" && len(emails) == 0:
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "add a tester group or at least one tester email")
		return
	case !accountOK:
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "service_account must be a JSON key with project_id, client_email, and private_key")
		return
	}
	rawEmails, _ := json.Marshal(emails)
	next := &store.FirebaseDistribution{
		Name:         name,
		AppID:        appID,
		ProjectID:    labels.ProjectID,
		ClientEmail:  labels.ClientEmail,
		GroupAlias:   group,
		TesterEmails: string(rawEmails),
	}
	var ct, nonce []byte
	replace := strings.TrimSpace(body.ServiceAccount) != ""
	if replace {
		var err error
		ct, nonce, err = s.Secrets.Seal([]byte(body.ServiceAccount))
		if err != nil {
			writeV1Err(w, http.StatusInternalServerError, "internal", "could not encrypt service account")
			return
		}
	}
	c, found, err := s.Store.UpdateFirebaseDistribution(r.Context(), tenantToken(r), r.PathValue("id"), next, ct, nonce, replace)
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
	s.auditFirebase(r, "firebase_distribution.updated", c.ID, c.Name)
	writeJSON(w, http.StatusOK, toV1FirebaseDistribution(c))
}

// DELETE /v1/firebase-distributions/{id}
func (s *Server) v1DeleteFirebaseDistribution(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil {
		writeV1Err(w, http.StatusServiceUnavailable, "unavailable", "firebase distribution store not configured")
		return
	}
	id := r.PathValue("id")
	deleted, err := s.Store.DeleteFirebaseDistribution(r.Context(), tenantToken(r), id)
	if err != nil {
		writeV1Err(w, http.StatusInternalServerError, "internal", "could not delete connection")
		return
	}
	if !deleted {
		writeV1Err(w, http.StatusNotFound, "not_found", "no such connection")
		return
	}
	s.auditFirebase(r, "firebase_distribution.deleted", id, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) auditFirebase(r *http.Request, action, id, name string) {
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

func readFirebaseAccount(raw string) (firebaseAccountLabels, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxFirebaseAccountBytes {
		return firebaseAccountLabels{}, false
	}
	var parsed struct {
		ProjectID   string `json:"project_id"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	if json.Unmarshal([]byte(raw), &parsed) != nil {
		return firebaseAccountLabels{}, false
	}
	parsed.ProjectID = strings.TrimSpace(parsed.ProjectID)
	parsed.ClientEmail = strings.TrimSpace(parsed.ClientEmail)
	if parsed.ProjectID == "" || !strings.Contains(parsed.ClientEmail, "@") || !strings.Contains(parsed.PrivateKey, "PRIVATE KEY") {
		return firebaseAccountLabels{}, false
	}
	return firebaseAccountLabels{ProjectID: parsed.ProjectID, ClientEmail: parsed.ClientEmail}, true
}

func normalizeFirebaseEmails(in []string) ([]string, bool) {
	if len(in) > 50 {
		return nil, false
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		addr := strings.ToLower(strings.TrimSpace(raw))
		if addr == "" {
			continue
		}
		parsed, err := mail.ParseAddress(addr)
		if err != nil || parsed.Address != addr || strings.ContainsAny(addr, "\x00\r\n") {
			return nil, false
		}
		if seen[addr] {
			continue
		}
		seen[addr] = true
		out = append(out, addr)
	}
	return out, true
}
