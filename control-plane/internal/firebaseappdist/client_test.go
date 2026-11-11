package firebaseappdist

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublishUploadsAndInvitesTesters(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	account, err := json.Marshal(map[string]string{
		"client_email": "distro@example.iam.gserviceaccount.com",
		"private_key":  pemKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	var sawUpload, sawPoll, sawInvite bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.URL.Path == "/token":
			if !strings.Contains(string(body), "assertion=") || strings.Contains(string(body), "PRIVATE KEY") {
				t.Errorf("token request leaked the key or omitted the assertion")
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "ya29.test"})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/releases:upload"):
			if !strings.Contains(r.URL.RequestURI(), "1%3A123%3Aandroid%3Aabcdef") && !strings.Contains(r.URL.Path, "1:123:android:abcdef") {
				t.Errorf("upload url = %s", r.URL.RequestURI())
			}
			sawUpload = true
			if r.Header.Get("X-Goog-Upload-Protocol") != "raw" || r.Header.Get("Authorization") != "Bearer ya29.test" {
				t.Errorf("upload headers = %v", r.Header)
			}
			if string(body) != "apk-bytes" {
				t.Errorf("upload body = %q", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "projects/123/apps/1:123:android:abcdef/releases/-/operations/op1",
				"done": false,
			})
		case strings.Contains(r.URL.Path, "/operations/op1"):
			sawPoll = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "projects/123/apps/1:123:android:abcdef/releases/-/operations/op1",
				"done": true,
				"response": map[string]any{
					"@type":  "type.googleapis.com/google.firebase.appdistribution.v1.UploadReleaseResponse",
					"result": "RELEASE_CREATED",
					"release": map[string]string{
						"name":               "projects/123/apps/1:123:android:abcdef/releases/rel1",
						"testingUri":         "https://appdistribution.firebase.google.com/testerapps/1:123:android:abcdef/releases/rel1",
						"firebaseConsoleUri": "https://console.firebase.google.com/project/example/appdistribution",
					},
				},
			})
		case strings.HasSuffix(r.URL.Path, ":distribute"):
			sawInvite = true
			var payload struct {
				TesterEmails []string `json:"testerEmails"`
				GroupAliases []string `json:"groupAliases"`
			}
			_ = json.Unmarshal(body, &payload)
			if len(payload.TesterEmails) != 1 || payload.TesterEmails[0] != "ada@example.com" || len(payload.GroupAliases) != 1 || payload.GroupAliases[0] != "testers" {
				t.Errorf("invite payload = %+v", payload)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	rel, err := (&Client{HTTP: srv.Client(), TokenURL: srv.URL + "/token", APIRoot: srv.URL}).Publish(context.Background(), Request{
		AccountJSON:  account,
		AppID:        "1:123:android:abcdef",
		APK:          []byte("apk-bytes"),
		GroupAlias:   "testers",
		TesterEmails: []string{"ada@example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawUpload || !sawPoll || !sawInvite {
		t.Fatalf("upload=%v poll=%v invite=%v", sawUpload, sawPoll, sawInvite)
	}
	if rel.TestingURI == "" || !strings.Contains(rel.TestingURI, "testerapps") {
		t.Fatalf("release = %+v", rel)
	}
}

func TestDecodeReleaseReadsTheUploadResponse(t *testing.T) {
	rel, err := decodeRelease([]byte(`{"result":"RELEASE_UNMODIFIED","release":{"name":"projects/1/apps/1:1:android:ab/releases/rel1","testingUri":"https://appdistribution.firebase.google.com/testerapps/example"}}`))
	if err != nil || !strings.Contains(rel.Name, "/releases/rel1") || rel.TestingURI == "" {
		t.Fatalf("rel=%+v err=%v", rel, err)
	}
	flat, err := decodeRelease([]byte(`{"name":"projects/1/apps/1:1:android:ab/releases/rel2","testingUri":"https://appdistribution.firebase.google.com/testerapps/example2"}`))
	if err != nil || !strings.Contains(flat.Name, "/releases/rel2") {
		t.Fatalf("flat=%+v err=%v", flat, err)
	}
}

func TestPublishRejectsABadAppIDWithoutCallingFirebase(t *testing.T) {
	_, err := New().Publish(context.Background(), Request{AccountJSON: []byte(`{"client_email":"a@b.c","private_key":"x"}`), AppID: "not-an-app"})
	if err == nil || !strings.Contains(err.Error(), "app id") {
		t.Fatalf("err = %v", err)
	}
}
