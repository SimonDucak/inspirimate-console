// Package firebaseappdist uploads an APK to Firebase App Distribution and
// invites the configured testers. The service account JSON is used only to
// mint a short-lived access token and is never written to a log.
package firebaseappdist

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultTokenURL = "https://oauth2.googleapis.com/token"
	defaultAPIRoot  = "https://firebaseappdistribution.googleapis.com"
	cloudPlatform   = "https://www.googleapis.com/auth/cloud-platform"
)

// Request is one APK release for one Firebase Android app.
type Request struct {
	AccountJSON  []byte
	AppID        string
	APK          []byte
	FileName     string
	GroupAlias   string
	TesterEmails []string
}

// Release is the Firebase release testers install.
type Release struct {
	Name       string
	TestingURI string
	ConsoleURI string
}

// Client talks to the Firebase App Distribution REST API.
type Client struct {
	HTTP     *http.Client
	TokenURL string
	APIRoot  string
	Now      func() time.Time
}

// New returns a client with a five-minute timeout for the APK upload.
func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 5 * time.Minute}}
}

type account struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

type operation struct {
	Name     string          `json:"name"`
	Done     bool            `json:"done"`
	Error    *opError        `json:"error"`
	Response json.RawMessage `json:"response"`
}

type opError struct {
	Message string `json:"message"`
}

type releaseBody struct {
	Name       string `json:"name"`
	TestingURI string `json:"testingUri"`
	ConsoleURI string `json:"firebaseConsoleUri"`
}

// Publish uploads the APK and distributes it to the group and emails.
func (c *Client) Publish(ctx context.Context, in Request) (Release, error) {
	if c == nil {
		c = New()
	}
	var acct account
	if err := json.Unmarshal(in.AccountJSON, &acct); err != nil {
		return Release{}, errors.New("service account JSON could not be read")
	}
	number, err := projectNumber(in.AppID)
	if err != nil {
		return Release{}, err
	}
	token, err := c.accessToken(ctx, acct)
	if err != nil {
		return Release{}, err
	}
	op, err := c.upload(ctx, token, number, in.AppID, in.FileName, in.APK)
	if err != nil {
		return Release{}, err
	}
	op, err = c.wait(ctx, token, op)
	if err != nil {
		return Release{}, err
	}
	rel, err := decodeRelease(op.Response)
	if err != nil {
		return Release{}, err
	}
	if err := c.distribute(ctx, token, rel.Name, in.GroupAlias, in.TesterEmails); err != nil {
		return rel, err
	}
	return rel, nil
}

func projectNumber(appID string) (string, error) {
	parts := strings.Split(appID, ":")
	if len(parts) != 4 || parts[0] != "1" || parts[2] != "android" || !digits(parts[1]) || parts[3] == "" {
		return "", errors.New("firebase android app id must look like 1:123:android:abc")
	}
	return parts[1], nil
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (c *Client) accessToken(ctx context.Context, acct account) (string, error) {
	key, err := parsePrivateKey(acct.PrivateKey)
	if err != nil {
		return "", err
	}
	endpoint := c.tokenEndpoint(acct)
	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	assertion, err := signJWT(key, acct.ClientEmail, endpoint, now)
	if err != nil {
		return "", err
	}
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.http().Do(req)
	if err != nil {
		return "", fmt.Errorf("firebase token request failed: %s", redact(err.Error()))
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 8192))
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("firebase rejected the service account (%d): %s", res.StatusCode, googleMessage(body))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return "", errors.New("firebase token response had no access token")
	}
	return tok.AccessToken, nil
}

func (c *Client) tokenEndpoint(acct account) string {
	if c.TokenURL != "" {
		return c.TokenURL
	}
	if strings.HasPrefix(acct.TokenURI, "https://") {
		return acct.TokenURI
	}
	return defaultTokenURL
}

func (c *Client) upload(ctx context.Context, token, projectNumber, appID, fileName string, apk []byte) (operation, error) {
	if fileName == "" {
		fileName = "app-release.apk"
	}
	app := "projects/" + projectNumber + "/apps/" + appID
	endpoint := c.root() + "/upload/v1/" + escapeResource(app) + "/releases:upload"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(apk))
	if err != nil {
		return operation{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Goog-Upload-Protocol", "raw")
	req.Header.Set("X-Goog-Upload-File-Name", fileName)
	res, err := c.http().Do(req)
	if err != nil {
		return operation{}, fmt.Errorf("firebase upload failed: %s", redact(err.Error()))
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return operation{}, fmt.Errorf("firebase upload was rejected (%d): %s", res.StatusCode, googleMessage(body))
	}
	var op operation
	if err := json.Unmarshal(body, &op); err != nil {
		return operation{}, errors.New("firebase upload response could not be read")
	}
	if op.Error != nil && op.Error.Message != "" {
		return operation{}, fmt.Errorf("firebase upload failed: %s", redact(op.Error.Message))
	}
	return op, nil
}

func (c *Client) wait(ctx context.Context, token string, op operation) (operation, error) {
	deadline := time.Now().Add(2 * time.Minute)
	for {
		if op.Error != nil && op.Error.Message != "" {
			return operation{}, fmt.Errorf("firebase upload failed: %s", redact(op.Error.Message))
		}
		if op.Done {
			return op, nil
		}
		if op.Name == "" {
			return operation{}, errors.New("firebase upload did not return a release")
		}
		if time.Now().After(deadline) {
			return operation{}, errors.New("firebase upload did not finish")
		}
		next, err := c.getOperation(ctx, token, op.Name)
		if err != nil {
			return operation{}, err
		}
		op = next
		if op.Done || (op.Error != nil && op.Error.Message != "") {
			continue
		}
		select {
		case <-ctx.Done():
			return operation{}, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func (c *Client) getOperation(ctx context.Context, token, name string) (operation, error) {
	endpoint := c.root() + "/v1/" + escapeResource(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return operation{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := c.http().Do(req)
	if err != nil {
		return operation{}, fmt.Errorf("firebase upload status failed: %s", redact(err.Error()))
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return operation{}, fmt.Errorf("firebase upload status was rejected (%d): %s", res.StatusCode, googleMessage(body))
	}
	var op operation
	if err := json.Unmarshal(body, &op); err != nil {
		return operation{}, errors.New("firebase upload status could not be read")
	}
	return op, nil
}

func (c *Client) distribute(ctx context.Context, token, releaseName, group string, emails []string) error {
	body := map[string]any{}
	if len(emails) > 0 {
		body["testerEmails"] = emails
	}
	if group != "" {
		body["groupAliases"] = []string{group}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	endpoint := c.root() + "/v1/" + escapeResource(releaseName) + ":distribute"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http().Do(req)
	if err != nil {
		return fmt.Errorf("firebase invite failed: %s", redact(err.Error()))
	}
	defer res.Body.Close()
	resp, _ := io.ReadAll(io.LimitReader(res.Body, 8192))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("firebase did not invite the testers (%d): %s", res.StatusCode, googleMessage(resp))
	}
	return nil
}

func decodeRelease(raw json.RawMessage) (Release, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return Release{}, errors.New("firebase upload finished without a release")
	}
	// releases:upload completes as an Operation whose response is an
	// UploadReleaseResponse: {"result":"RELEASE_CREATED","release":{...}}.
	// A bare Release is accepted too.
	var body struct {
		Result     string       `json:"result"`
		Release    *releaseBody `json:"release"`
		Name       string       `json:"name"`
		TestingURI string       `json:"testingUri"`
		ConsoleURI string       `json:"firebaseConsoleUri"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return Release{}, errors.New("firebase upload finished without a release")
	}
	rel := releaseBody{Name: body.Name, TestingURI: body.TestingURI, ConsoleURI: body.ConsoleURI}
	if body.Release != nil && body.Release.Name != "" {
		rel = *body.Release
	}
	if rel.Name == "" {
		if body.Result != "" {
			return Release{}, fmt.Errorf("firebase upload finished without a release (%s)", body.Result)
		}
		return Release{}, errors.New("firebase upload finished without a release")
	}
	return Release{Name: rel.Name, TestingURI: rel.TestingURI, ConsoleURI: rel.ConsoleURI}, nil
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *Client) root() string {
	if c.APIRoot != "" {
		return strings.TrimRight(c.APIRoot, "/")
	}
	return defaultAPIRoot
}

func escapeResource(name string) string {
	parts := strings.Split(name, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func parsePrivateKey(pemText string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, errors.New("service account private key could not be read")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("service account private key could not be read")
		}
		return rsaKey, nil
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("service account private key could not be read")
	}
	return key, nil
}

func signJWT(key *rsa.PrivateKey, email, audience string, now time.Time) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]any{
		"iss":   email,
		"scope": cloudPlatform,
		"aud":   audience,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(claims)
	sum := sha256.Sum256([]byte(header + "." + payload))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", errors.New("service account private key could not be read")
	}
	return header + "." + payload + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func googleMessage(body []byte) string {
	var wrapped struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &wrapped) == nil {
		if wrapped.Error.Message != "" {
			return redact(wrapped.Error.Message)
		}
		if wrapped.Message != "" {
			return redact(wrapped.Message)
		}
	}
	return redact(string(body))
}

func redact(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "PRIVATE KEY"); i >= 0 {
		s = strings.TrimSpace(s[:i]) + " [redacted]"
	}
	if len(s) > 300 {
		s = s[:300]
	}
	if s == "" {
		return "empty response"
	}
	return s
}
