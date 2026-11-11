package api

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/tastyeffectco/sandboxd/control-plane/internal/firebaseappdist"
	"github.com/tastyeffectco/sandboxd/control-plane/internal/secrets"
	"github.com/tastyeffectco/sandboxd/control-plane/internal/store"
)

type recordingFirebase struct {
	called bool
	appID  string
	emails []string
	group  string
}

func (r *recordingFirebase) Publish(_ context.Context, in firebaseappdist.Request) (firebaseappdist.Release, error) {
	r.called = true
	r.appID = in.AppID
	r.emails = in.TesterEmails
	r.group = in.GroupAlias
	return firebaseappdist.Release{TestingURI: "https://appdistribution.firebase.google.com/testerapps/example", ConsoleURI: "https://console.firebase.google.com/example"}, nil
}

func TestPublishApkSendsOnlyWhenTheProjectIsLinked(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "file::memory:?_fk=1", filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := secrets.Load("", filepath.Join(t.TempDir(), "secrets.key"))
	if err != nil {
		t.Fatal(err)
	}
	s, id := apkTestServer(t)
	s.Store = st
	s.Secrets = cipher
	fake := &recordingFirebase{}
	s.Firebase = fake
	writeApkFile(t, apkCanonicalPath(s.appDirFor(id)), "apk-bytes")
	if err := saveApkHistory(s.appDirFor(id), apkHistory{Builds: []apkBuild{{
		ID: "build1", Status: apkStatusSucceeded, StartedAt: "2020-01-02T00:00:00Z", Version: "1.0.0",
	}}}); err != nil {
		t.Fatal(err)
	}

	s.publishApk(id, "build1", "1.0.0")
	if fake.called {
		t.Fatal("publish ran without a project connection")
	}
	skipped := loadApkHistory(s.appDirFor(id)).Builds[0]
	if skipped.FirebaseStatus != apkFirebaseSkipped {
		t.Fatalf("status = %s", skipped.FirebaseStatus)
	}

	app := &store.App{ID: "01APPFIREBASEPUBLISH000001", OwnerToken: "default", Name: "Coach"}
	if err := st.CreateApp(ctx, app); err != nil {
		t.Fatal(err)
	}
	account := []byte(`{"project_id":"coach","client_email":"distro@coach.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n"}`)
	enc, nonce, err := cipher.Seal(account)
	if err != nil {
		t.Fatal(err)
	}
	conn := &store.FirebaseDistribution{
		ID: "01FIREBASEPUBLISH00000001", OwnerToken: "default", Name: "Android",
		AppID: "1:566195963885:android:abcd", ProjectID: "coach", ClientEmail: "distro@coach.iam.gserviceaccount.com",
		GroupAlias: "testers", TesterEmails: `["ada@example.com"]`,
	}
	if err := st.CreateFirebaseDistribution(ctx, conn, enc, nonce); err != nil {
		t.Fatal(err)
	}
	link := conn.ID
	if err := st.UpdateApp(ctx, app.ID, "default", store.AppPatch{FirebaseDistributionID: &link}); err != nil {
		t.Fatal(err)
	}
	if err := st.Create(ctx, &store.Sandbox{
		ID: id, Status: "running", AppID: sql.NullString{String: app.ID, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}

	s.publishApk(id, "build1", "1.0.0")
	if !fake.called || fake.appID != conn.AppID || fake.group != "testers" || len(fake.emails) != 1 {
		t.Fatalf("publish = %+v", fake)
	}
	sent := loadApkHistory(s.appDirFor(id)).Builds[0]
	if sent.FirebaseStatus != apkFirebaseSucceeded || sent.TestingURI == "" {
		t.Fatalf("result = %+v", sent)
	}
}
