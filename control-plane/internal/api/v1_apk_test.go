package api

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tastyeffectco/sandboxd/control-plane/internal/docker"
	"github.com/tastyeffectco/sandboxd/control-plane/internal/loopback"
)

type scriptedApkExec struct {
	fn func() (docker.ExecResult, error)
}

func (s scriptedApkExec) ExecTail(context.Context, string, []string, int) (docker.ExecResult, error) {
	return s.fn()
}

func apkTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	s := &Server{
		Loopback: &loopback.Manager{Root: root},
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	id := newULID()
	if err := os.MkdirAll(s.appDirFor(id), 0o755); err != nil {
		t.Fatal(err)
	}
	return s, id
}

func writeApkFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestApkSecondSuccessReplacesFile(t *testing.T) {
	s, id := apkTestServer(t)
	app := s.appDirFor(id)
	writeApkFile(t, apkCanonicalPath(app), "old-apk")
	writeApkFile(t, apkFlutterOutput(app), "new-apk")
	if err := saveApkHistory(app, apkHistory{Builds: []apkBuild{{
		ID: "old", Status: apkStatusSucceeded, StartedAt: "2020-01-01T00:00:00Z",
	}, {
		ID: "new", Status: apkStatusRunning, StartedAt: "2020-01-02T00:00:00Z", Version: "1.2.0",
	}}}); err != nil {
		t.Fatal(err)
	}

	s.finishApkBuild(id, "new", docker.ExecResult{ExitCode: 0}, nil)

	got, err := os.ReadFile(apkCanonicalPath(app))
	if err != nil || string(got) != "new-apk" {
		t.Fatalf("canonical apk = %q err=%v", got, err)
	}
	if _, err := os.Stat(apkFlutterOutput(app)); !os.IsNotExist(err) {
		t.Fatalf("flutter output still present: %v", err)
	}
	views := apkViews(loadApkHistory(app).Builds)
	if len(views) != 2 || views[0].ID != "new" || !views[0].Downloadable || views[0].Version != "1.2.0" {
		t.Fatalf("newest success not downloadable: %+v", views)
	}
	if views[1].Downloadable {
		t.Fatal("previous success stayed downloadable")
	}
}

func TestApkFailureKeepsPreviousFile(t *testing.T) {
	s, id := apkTestServer(t)
	app := s.appDirFor(id)
	writeApkFile(t, apkCanonicalPath(app), "old-apk")
	writeApkFile(t, apkFlutterOutput(app), "partial")
	if err := saveApkHistory(app, apkHistory{Builds: []apkBuild{{
		ID: "good", Status: apkStatusSucceeded, StartedAt: "2020-01-01T00:00:00Z",
	}, {
		ID: "bad", Status: apkStatusRunning, StartedAt: "2020-01-02T00:00:00Z",
	}}}); err != nil {
		t.Fatal(err)
	}

	s.finishApkBuild(id, "bad", docker.ExecResult{ExitCode: 1, Stderr: "gradle failed"}, nil)

	got, err := os.ReadFile(apkCanonicalPath(app))
	if err != nil || string(got) != "old-apk" {
		t.Fatalf("canonical apk = %q err=%v", got, err)
	}
	views := apkViews(loadApkHistory(app).Builds)
	if views[0].ID != "bad" || views[0].Status != apkStatusFailed || views[0].Downloadable {
		t.Fatalf("failed row: %+v", views[0])
	}
	if views[1].ID != "good" || !views[1].Downloadable {
		t.Fatalf("last good apk not downloadable: %+v", views[1])
	}
}

func TestDownloadServesOnlyLatestApk(t *testing.T) {
	s, id := apkTestServer(t)
	app := s.appDirFor(id)

	if w := downloadApk(s, id); w.Code != http.StatusNotFound {
		t.Fatalf("empty download code=%d", w.Code)
	}

	writeApkFile(t, apkCanonicalPath(app), "old-apk")
	if err := saveApkHistory(app, apkHistory{Builds: []apkBuild{{
		ID: "good", Status: apkStatusSucceeded, StartedAt: "2020-01-01T00:00:00Z",
	}}}); err != nil {
		t.Fatal(err)
	}
	w := downloadApk(s, id)
	if w.Code != http.StatusOK || w.Body.String() != "old-apk" {
		t.Fatalf("download code=%d body=%q type=%q", w.Code, w.Body.String(), w.Header().Get("Content-Type"))
	}
	if w.Header().Get("Content-Type") != "application/vnd.android.package-archive" {
		t.Fatalf("content type %q", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), "app-release.apk") {
		t.Fatalf("disposition %q", w.Header().Get("Content-Disposition"))
	}

	writeApkFile(t, apkFlutterOutput(app), "new-apk")
	if err := saveApkHistory(app, apkHistory{Builds: []apkBuild{
		{ID: "good", Status: apkStatusSucceeded, StartedAt: "2020-01-01T00:00:00Z"},
		{ID: "next", Status: apkStatusRunning, StartedAt: "2020-01-02T00:00:00Z"},
	}}); err != nil {
		t.Fatal(err)
	}
	s.finishApkBuild(id, "next", docker.ExecResult{}, nil)
	if w := downloadApk(s, id); w.Body.String() != "new-apk" {
		t.Fatalf("download after replace = %q", w.Body.String())
	}
}

func downloadApk(s *Server, id string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/v1/sandboxes/"+id+"/apk-builds/latest", nil)
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	s.v1DownloadApk(w, r)
	return w
}

func TestApkBuildRejectsASecondStart(t *testing.T) {
	s, id := apkTestServer(t)
	block := make(chan struct{})
	s.ApkExec = scriptedApkExec{fn: func() (docker.ExecResult, error) {
		<-block
		return docker.ExecResult{ExitCode: 1}, nil
	}}

	w1 := startApk(t, s, id, `{"version":"9"}`)
	if w1.Code != http.StatusAccepted {
		t.Fatalf("start code=%d body=%s", w1.Code, w1.Body.String())
	}
	waitApkStatus(t, s, id, apkStatusRunning)

	w2 := startApk(t, s, id, `{}`)
	if w2.Code != http.StatusConflict {
		t.Fatalf("second start code=%d body=%s", w2.Code, w2.Body.String())
	}
	close(block)
	waitApkStatus(t, s, id, apkStatusFailed)
}

func startApk(t *testing.T, s *Server, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/v1/sandboxes/"+id+"/apk-builds", strings.NewReader(body))
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	s.v1StartApkBuild(w, r)
	return w
}

func waitApkStatus(t *testing.T, s *Server, id, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		views := apkViews(loadApkHistory(s.appDirFor(id)).Builds)
		if len(views) > 0 && views[0].Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("status did not become %s: %+v", want, loadApkHistory(s.appDirFor(id)))
}

func TestCapGradlePropertiesRestores(t *testing.T) {
	app := t.TempDir()
	path := filepath.Join(app, "android", "gradle.properties")
	orig := "org.gradle.jvmargs=-Xmx8G\nandroid.useAndroidX=true\n"
	writeApkFile(t, path, orig)

	restore, err := capGradleProperties(app)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{
		"org.gradle.jvmargs=-Xmx1024m -XX:MaxMetaspaceSize=768m -Djava.io.tmpdir=/home/sandbox/.gradle/tmp",
		"org.gradle.workers.max=1",
		"org.gradle.daemon=false",
		"org.gradle.vfs.watch=false",
		"systemProp.org.gradle.native=false",
		"android.useAndroidX=true",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("capped file missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "-Xmx8G") {
		t.Fatalf("old heap still present:\n%s", text)
	}
	restore()
	after, err := os.ReadFile(path)
	if err != nil || string(after) != orig {
		t.Fatalf("restored %q err=%v", after, err)
	}
}

func TestApkLintSkipIsRemovedAfterBuildSetup(t *testing.T) {
	app := filepath.Join(t.TempDir(), "workspace", "app")
	restore, err := installApkLintSkip(app)
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(filepath.Dir(filepath.Dir(app)), ".gradle", "init.d", "sandboxd-apk-lint.gradle")
	body, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "checkReleaseBuilds = false") {
		t.Fatalf("init script missing lint skip:\n%s", body)
	}
	gradleHome := filepath.Dir(filepath.Dir(script))
	for _, p := range []string{gradleHome, filepath.Dir(script), filepath.Join(gradleHome, "tmp"), script} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		uid := st.Sys().(*syscall.Stat_t).Uid
		if uid != 1000 {
			t.Fatalf("%s owned by uid %d; sandbox user is 1000", p, uid)
		}
	}
	restore()
	if _, err := os.Stat(script); !os.IsNotExist(err) {
		t.Fatalf("init script still present: %v", err)
	}
}

func TestReconcileApkBuildMarksInterrupted(t *testing.T) {
	s, id := apkTestServer(t)
	app := s.appDirFor(id)
	if err := saveApkHistory(app, apkHistory{Builds: []apkBuild{{
		ID: "stuck", Status: apkStatusRunning, StartedAt: "2020-01-01T00:00:00Z",
	}}}); err != nil {
		t.Fatal(err)
	}
	s.ReconcileApkBuilds()
	h := loadApkHistory(app)
	if len(h.Builds) != 1 || h.Builds[0].Status != apkStatusFailed || h.Builds[0].Log != apkInterruptedLog {
		t.Fatalf("reconcile: %+v", h.Builds)
	}
}

func TestSandboxdMetaHiddenFromFilesAndExport(t *testing.T) {
	s, id := apkTestServer(t)
	app := s.appDirFor(id)
	writeApkFile(t, filepath.Join(app, "lib", "main.dart"), "void main() {}")
	writeApkFile(t, apkCanonicalPath(app), "apk-bytes")

	r := httptest.NewRequest(http.MethodGet, "/v1/sandboxes/"+id+"/files?recursive=true", nil)
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()
	s.v1ListFiles(w, r)
	if strings.Contains(w.Body.String(), ".sandboxd") || strings.Contains(w.Body.String(), "app-release.apk") {
		t.Fatalf("listing exposed apk metadata: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "main.dart") {
		t.Fatalf("listing dropped source: %s", w.Body.String())
	}

	er := httptest.NewRequest(http.MethodGet, "/v1/sandboxes/"+id+"/export", nil)
	er.SetPathValue("id", id)
	ew := httptest.NewRecorder()
	s.v1Export(ew, er)
	zr, err := zip.NewReader(bytes.NewReader(ew.Body.Bytes()), int64(ew.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if strings.Contains(f.Name, ".sandboxd") {
			t.Fatalf("export included %s", f.Name)
		}
	}
}
