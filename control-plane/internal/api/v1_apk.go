package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tastyeffectco/sandboxd/control-plane/internal/docker"
	"github.com/tastyeffectco/sandboxd/control-plane/internal/firebaseappdist"
	"github.com/tastyeffectco/sandboxd/control-plane/internal/store"
)

// apkExecer runs a command in a sandbox and keeps only the tail of its
// output. *docker.Client implements it; tests inject a fake.
type apkExecer interface {
	ExecTail(ctx context.Context, name string, cmd []string, maxOutput int) (docker.ExecResult, error)
}

const (
	apkStatusRunning   = "running"
	apkStatusSucceeded = "succeeded"
	apkStatusFailed    = "failed"

	apkFirebaseRunning   = "running"
	apkFirebaseSucceeded = "succeeded"
	apkFirebaseFailed    = "failed"
	apkFirebaseSkipped   = "skipped"

	apkLogLimit     = 8 << 10
	apkCaptureLimit = 32 << 10
	apkHistoryLimit = 50

	apkInterruptedLog = "interrupted by a control-plane restart"

	// Login shell so /etc/profile.d/flutter.sh puts flutter and the Android
	// SDK on PATH. The project directory is the bind-mounted app workspace.
	// /tmp is mounted noexec, and Gradle extracts libgradle-fileevents.so
	// there, so the kernel refuses to map it ("Failed to initialise native
	// integration"). The build uses an executable temp dir under the home
	// directory instead. GRADLE_OPTS covers the Gradle client; the daemon
	// picks up java.io.tmpdir from org.gradle.jvmargs below.
	apkBuildShell = `mkdir -p /home/sandbox/.gradle/tmp && cd /home/sandbox/workspace/app && export TMPDIR=/home/sandbox/.gradle/tmp JAVA_TOOL_OPTIONS=-Djava.io.tmpdir=/home/sandbox/.gradle/tmp GRADLE_OPTS="${GRADLE_OPTS:-} -Djava.io.tmpdir=/home/sandbox/.gradle/tmp -Dorg.gradle.native=false -Dorg.gradle.vfs.watch=false" && flutter build apk --release`
)

// gradleCaps are applied to android/gradle.properties for the duration of
// one APK build, then the file is restored. A 4 GB host cannot survive
// Flutter's default multi-gigabyte Gradle heap, and an already-running
// daemon would ignore a new heap size, so the daemon is turned off too.
// java.io.tmpdir is in the daemon's own JVM args: /tmp is noexec.
// 256 MB of metaspace is not enough for Android lint on a real plugin
// set (the JVM dies with "Metaspace"). Release lint is skipped by an
// init script; 768 MB is the room left for the shrinker.
var gradleCaps = [][2]string{
	{"org.gradle.jvmargs", "-Xmx1024m -XX:MaxMetaspaceSize=768m -Djava.io.tmpdir=/home/sandbox/.gradle/tmp"},
	{"org.gradle.workers.max", "1"},
	{"org.gradle.daemon", "false"},
	{"org.gradle.vfs.watch", "false"},
	{"systemProp.org.gradle.native", "false"},
}

type apkBuild struct {
	ID                 string `json:"id"`
	Version            string `json:"version,omitempty"`
	Status             string `json:"status"`
	StartedAt          string `json:"started_at"`
	FinishedAt         string `json:"finished_at,omitempty"`
	Log                string `json:"log,omitempty"`
	FirebaseStatus     string `json:"firebase_status,omitempty"`
	FirebaseLog        string `json:"firebase_log,omitempty"`
	TestingURI         string `json:"testing_uri,omitempty"`
	FirebaseConsoleURI string `json:"firebase_console_uri,omitempty"`
}

// apkFirebasePublisher uploads one APK and invites testers. *firebaseappdist.Client
// implements it.
type apkFirebasePublisher interface {
	Publish(ctx context.Context, in firebaseappdist.Request) (firebaseappdist.Release, error)
}

type apkBuildView struct {
	apkBuild
	Downloadable bool `json:"downloadable"`
}

type apkHistory struct {
	Builds []apkBuild `json:"builds"`
}

func (s *Server) apkExec() apkExecer {
	if s.ApkExec != nil {
		return s.ApkExec
	}
	if s.Docker == nil {
		return nil
	}
	return s.Docker
}

func apkHistoryPath(appDir string) string {
	return filepath.Join(appDir, ".sandboxd", "apk-builds.json")
}

func apkCanonicalPath(appDir string) string {
	return filepath.Join(appDir, ".sandboxd", "apk", "app-release.apk")
}

func apkFlutterOutput(appDir string) string {
	return filepath.Join(appDir, "build", "app", "outputs", "flutter-apk", "app-release.apk")
}

func (s *Server) workspaceReady(id string) bool {
	if s.Loopback == nil {
		return false
	}
	_, mnt := s.Loopback.Paths(id)
	info, err := os.Stat(mnt)
	return err == nil && info.IsDir()
}

// ReconcileApkBuilds marks APK builds left "running" by a previous process
// as failed. Nothing in this process is running yet, so every running row
// is stale. Called once at startup.
func (s *Server) ReconcileApkBuilds() {
	if s.Loopback == nil || s.Loopback.Root == "" {
		return
	}
	ents, err := os.ReadDir(s.Loopback.Root)
	if err != nil {
		return
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		app := filepath.Join(s.Loopback.Root, e.Name(), appSubdir)
		s.apkMu.Lock()
		s.failStaleApkLocked(app, e.Name())
		s.apkMu.Unlock()
	}
}

// failStaleApkLocked marks running history rows failed when this process
// is not actually building them. Caller holds s.apkMu.
func (s *Server) failStaleApkLocked(appDir, sandboxID string) {
	live := ""
	if s.apkRunning != nil {
		live = s.apkRunning[sandboxID]
	}
	h := loadApkHistory(appDir)
	changed := false
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range h.Builds {
		b := &h.Builds[i]
		if b.ID == live {
			continue
		}
		if b.Status == apkStatusRunning {
			b.Status = apkStatusFailed
			b.FinishedAt = now
			if strings.TrimSpace(b.Log) == "" {
				b.Log = apkInterruptedLog
			}
			if b.FirebaseStatus == "" || b.FirebaseStatus == apkFirebaseRunning {
				b.FirebaseStatus = apkFirebaseSkipped
				b.FirebaseLog = "Skipped because the APK build failed."
			}
			changed = true
			continue
		}
		if b.FirebaseStatus == apkFirebaseRunning {
			b.FirebaseStatus = apkFirebaseFailed
			b.FirebaseLog = apkInterruptedLog
			changed = true
		}
	}
	if changed {
		_ = saveApkHistory(appDir, h)
	}
}

func loadApkHistory(appDir string) apkHistory {
	raw, err := os.ReadFile(apkHistoryPath(appDir))
	if err != nil {
		return apkHistory{}
	}
	var h apkHistory
	if json.Unmarshal(raw, &h) != nil {
		return apkHistory{}
	}
	return h
}

func saveApkHistory(appDir string, h apkHistory) error {
	sort.SliceStable(h.Builds, func(i, j int) bool {
		if h.Builds[i].StartedAt != h.Builds[j].StartedAt {
			return h.Builds[i].StartedAt > h.Builds[j].StartedAt
		}
		return h.Builds[i].ID > h.Builds[j].ID
	})
	if len(h.Builds) > apkHistoryLimit {
		h.Builds = h.Builds[:apkHistoryLimit]
	}
	raw, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	path := apkHistoryPath(appDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

func apkViews(builds []apkBuild) []apkBuildView {
	sorted := append([]apkBuild(nil), builds...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].StartedAt != sorted[j].StartedAt {
			return sorted[i].StartedAt > sorted[j].StartedAt
		}
		return sorted[i].ID > sorted[j].ID
	})
	out := make([]apkBuildView, len(sorted))
	marked := false
	for i, b := range sorted {
		dl := !marked && b.Status == apkStatusSucceeded
		if dl {
			marked = true
		}
		out[i] = apkBuildView{apkBuild: b, Downloadable: dl}
	}
	return out
}

func cleanApkVersion(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	if len(v) > 40 {
		return "", errors.New("version must be 40 characters or fewer")
	}
	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			return "", errors.New("version contains unsupported characters")
		}
	}
	return v, nil
}

func (s *Server) v1ListApkBuilds(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isULID(id) || !s.workspaceReady(id) {
		writeV1Err(w, http.StatusNotFound, "not_found", "no such sandbox")
		return
	}
	app := s.appDirFor(id)
	s.apkMu.Lock()
	s.failStaleApkLocked(app, id)
	h := loadApkHistory(app)
	s.apkMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"builds": apkViews(h.Builds)})
}

func (s *Server) v1StartApkBuild(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isULID(id) {
		writeV1Err(w, http.StatusNotFound, "not_found", "no such sandbox")
		return
	}
	var req struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeV1Err(w, http.StatusBadRequest, "invalid_request", "invalid json")
		return
	}
	version, err := cleanApkVersion(req.Version)
	if err != nil {
		writeV1Err(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if s.Store != nil {
		sb, gerr := s.Store.Get(r.Context(), id)
		if errors.Is(gerr, store.ErrNotFound) {
			writeV1Err(w, http.StatusNotFound, "not_found", "no such sandbox")
			return
		}
		if gerr != nil {
			writeV1Err(w, http.StatusInternalServerError, "internal", gerr.Error())
			return
		}
		if sb.Status != "running" {
			writeV1Err(w, http.StatusConflict, "conflict", "sandbox is not running")
			return
		}
	}
	if !s.workspaceReady(id) {
		writeV1Err(w, http.StatusNotFound, "not_found", "no workspace for that sandbox")
		return
	}
	if s.apkExec() == nil {
		writeV1Err(w, http.StatusServiceUnavailable, "unavailable", "apk builds are unavailable")
		return
	}

	app := s.appDirFor(id)
	build := apkBuild{
		ID:        newULID(),
		Version:   version,
		Status:    apkStatusRunning,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}

	s.apkMu.Lock()
	s.failStaleApkLocked(app, id)
	if s.apkRunning[id] != "" {
		s.apkMu.Unlock()
		writeV1Err(w, http.StatusConflict, "conflict", "an APK build is already running")
		return
	}
	h := loadApkHistory(app)
	h.Builds = append([]apkBuild{build}, h.Builds...)
	if err := saveApkHistory(app, h); err != nil {
		s.apkMu.Unlock()
		writeV1Err(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if s.apkRunning == nil {
		s.apkRunning = map[string]string{}
	}
	s.apkRunning[id] = build.ID
	s.apkMu.Unlock()

	go s.runApkBuild(id, build.ID)
	writeJSON(w, http.StatusAccepted, apkBuildView{apkBuild: build})
}

func (s *Server) v1DownloadApk(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isULID(id) || !s.workspaceReady(id) {
		writeV1Err(w, http.StatusNotFound, "not_found", "no such sandbox")
		return
	}
	app := s.appDirFor(id)
	s.apkMu.Lock()
	h := loadApkHistory(app)
	s.apkMu.Unlock()
	if !apkHasSuccess(h.Builds) {
		writeV1Err(w, http.StatusNotFound, "not_found", "no APK to download")
		return
	}
	full := apkCanonicalPath(app)
	full, ok := realpathWithin(full, app)
	if !ok {
		writeV1Err(w, http.StatusNotFound, "not_found", "no APK to download")
		return
	}
	f, err := os.Open(full)
	if err != nil {
		writeV1Err(w, http.StatusNotFound, "not_found", "no APK to download")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		writeV1Err(w, http.StatusNotFound, "not_found", "no APK to download")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", `attachment; filename="app-release.apk"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	_, _ = io.Copy(w, f)
}

func apkHasSuccess(builds []apkBuild) bool {
	for _, b := range builds {
		if b.Status == apkStatusSucceeded {
			return true
		}
	}
	return false
}

func (s *Server) runApkBuild(sandboxID, buildID string) {
	if s.Inflight != nil {
		s.Inflight.Enter(sandboxID)
		defer s.Inflight.Exit(sandboxID)
	}
	defer func() {
		s.apkMu.Lock()
		if s.apkRunning[sandboxID] == buildID {
			delete(s.apkRunning, sandboxID)
		}
		s.apkMu.Unlock()
	}()
	if s.Store != nil {
		_ = s.Store.BumpLastActive(context.Background(), sandboxID, time.Now().UTC())
		defer func() {
			_ = s.Store.BumpLastActive(context.Background(), sandboxID, time.Now().UTC())
		}()
	}

	app := s.appDirFor(sandboxID)
	restore, err := capGradleProperties(app)
	if restore != nil {
		defer restore()
	}
	if err != nil {
		s.finishApkBuild(sandboxID, buildID, docker.ExecResult{}, err)
		return
	}
	restoreLint, lintErr := installApkLintSkip(app)
	if restoreLint != nil {
		defer restoreLint()
	}
	if lintErr != nil {
		s.finishApkBuild(sandboxID, buildID, docker.ExecResult{}, lintErr)
		return
	}
	ex := s.apkExec()
	if ex == nil {
		s.finishApkBuild(sandboxID, buildID, docker.ExecResult{}, errors.New("apk builds are unavailable"))
		return
	}
	res, execErr := ex.ExecTail(context.Background(), "s-"+sandboxID, []string{"bash", "-lc", apkBuildShell}, apkCaptureLimit)
	s.finishApkBuild(sandboxID, buildID, res, execErr)
}

func (s *Server) finishApkBuild(sandboxID, buildID string, res docker.ExecResult, execErr error) {
	app := s.appDirFor(sandboxID)
	status := apkStatusSucceeded
	logText := apkOutputLog(res, execErr)
	if execErr != nil || res.ExitCode != 0 {
		status = apkStatusFailed
	} else if err := promoteApk(app); err != nil {
		status = apkStatusFailed
		logText = apkOutputLog(res, err)
	}

	version := ""
	s.apkMu.Lock()
	h := loadApkHistory(app)
	for i := range h.Builds {
		if h.Builds[i].ID != buildID {
			continue
		}
		h.Builds[i].Status = status
		h.Builds[i].FinishedAt = time.Now().UTC().Format(time.RFC3339)
		h.Builds[i].Log = strings.TrimSpace(logText)
		version = h.Builds[i].Version
		if status != apkStatusSucceeded {
			h.Builds[i].FirebaseStatus = apkFirebaseSkipped
			h.Builds[i].FirebaseLog = "Skipped because the APK build failed."
		} else {
			// Mark the upload running in the same write as the APK result.
			// Otherwise a status poll can see a finished APK, stop polling,
			// and miss the Firebase result.
			h.Builds[i].FirebaseStatus = apkFirebaseRunning
			h.Builds[i].FirebaseLog = "Uploading the APK to Firebase."
		}
		break
	}
	_ = saveApkHistory(app, h)
	s.apkMu.Unlock()
	if status == apkStatusSucceeded {
		s.publishApk(sandboxID, buildID, version)
	}
}

func (s *Server) firebasePublisher() apkFirebasePublisher {
	if s.Firebase != nil {
		return s.Firebase
	}
	return firebaseappdist.New()
}

// publishApk sends the canonical APK to the Firebase connection attached to
// the sandbox's project. A project with no connection records a skipped stage
// and leaves the APK download in place.
func (s *Server) publishApk(sandboxID, buildID, version string) {
	if s.Store == nil || s.Secrets == nil {
		s.setApkFirebase(sandboxID, buildID, apkFirebaseSkipped, "No Firebase distribution connection is attached to this project.", "", "")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	sb, err := s.Store.Get(ctx, sandboxID)
	if err != nil || !sb.AppID.Valid || sb.AppID.String == "" {
		s.setApkFirebase(sandboxID, buildID, apkFirebaseSkipped, "No Firebase distribution connection is attached to this project.", "", "")
		return
	}
	app, err := s.Store.GetApp(ctx, sb.AppID.String)
	if err != nil || !app.FirebaseDistributionID.Valid || app.FirebaseDistributionID.String == "" {
		s.setApkFirebase(sandboxID, buildID, apkFirebaseSkipped, "No Firebase distribution connection is attached to this project.", "", "")
		return
	}
	conn, found, err := s.Store.GetFirebaseDistribution(ctx, app.OwnerToken, app.FirebaseDistributionID.String)
	if err != nil || !found {
		s.setApkFirebase(sandboxID, buildID, apkFirebaseFailed, "The Firebase distribution connection on this project is missing.", "", "")
		return
	}
	enc, nonce, found, err := s.Store.GetFirebaseDistributionSecret(ctx, app.OwnerToken, conn.ID)
	if err != nil || !found {
		s.setApkFirebase(sandboxID, buildID, apkFirebaseFailed, "The Firebase service account could not be read.", "", "")
		return
	}
	account, err := s.Secrets.Open(enc, nonce)
	if err != nil {
		s.setApkFirebase(sandboxID, buildID, apkFirebaseFailed, "The Firebase service account could not be read.", "", "")
		return
	}
	apk, err := os.ReadFile(apkCanonicalPath(s.appDirFor(sandboxID)))
	if err != nil {
		s.setApkFirebase(sandboxID, buildID, apkFirebaseFailed, "The APK file is missing.", "", "")
		return
	}
	s.setApkFirebase(sandboxID, buildID, apkFirebaseRunning, "Uploading the APK to Firebase.", "", "")
	rel, err := s.firebasePublisher().Publish(ctx, firebaseappdist.Request{
		AccountJSON:  account,
		AppID:        conn.AppID,
		APK:          apk,
		FileName:     "app-release.apk",
		GroupAlias:   conn.GroupAlias,
		TesterEmails: decodeStringList(conn.TesterEmails),
	})
	if err != nil {
		s.setApkFirebase(sandboxID, buildID, apkFirebaseFailed, err.Error(), "", "")
		return
	}
	note := "Firebase emailed the invited testers."
	if version != "" {
		note = "Firebase emailed the invited testers. Version " + version + "."
	}
	s.setApkFirebase(sandboxID, buildID, apkFirebaseSucceeded, note, rel.TestingURI, rel.ConsoleURI)
}

func (s *Server) setApkFirebase(sandboxID, buildID, status, logText, testingURI, consoleURI string) {
	app := s.appDirFor(sandboxID)
	s.apkMu.Lock()
	defer s.apkMu.Unlock()
	h := loadApkHistory(app)
	for i := range h.Builds {
		if h.Builds[i].ID != buildID {
			continue
		}
		h.Builds[i].FirebaseStatus = status
		h.Builds[i].FirebaseLog = tailText(strings.TrimSpace(logText), apkLogLimit)
		h.Builds[i].TestingURI = testingURI
		h.Builds[i].FirebaseConsoleURI = consoleURI
		break
	}
	_ = saveApkHistory(app, h)
}

func apkOutputLog(res docker.ExecResult, extra error) string {
	var b strings.Builder
	b.WriteString(res.Stdout)
	if strings.TrimSpace(res.Stderr) != "" {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
		b.WriteString(res.Stderr)
	}
	if extra != nil {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
		b.WriteString(extra.Error())
	}
	return tailText(b.String(), apkLogLimit)
}

func tailText(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	cut := s[len(s)-n:]
	// A byte cut can split a rune. Drop the broken prefix so the history
	// JSON stays valid UTF-8 and the status update can be saved.
	for cut != "" && !utf8.ValidString(cut) {
		cut = cut[1:]
	}
	return "…\n" + cut
}

// promoteApk copies the freshly built APK over the one kept file and
// removes the copy Flutter left in the project build directory. A failed
// copy leaves the previous APK in place.
func promoteApk(appDir string) error {
	src := apkFlutterOutput(appDir)
	st, err := os.Stat(src)
	if err != nil || st.IsDir() {
		return errors.New("apk was not produced")
	}
	dst := apkCanonicalPath(appDir)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".next"
	if err := copyFile(src, tmp); err != nil {
		return err
	}
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Remove(src)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(dst)
		return copyErr
	}
	return closeErr
}

// capGradleProperties rewrites android/gradle.properties for one build.
// restore puts the original bytes back, or removes a file this call created.
// A missing android/ directory is left alone: Flutter creates that project
// itself, and a stub file would get in the way.
func capGradleProperties(appDir string) (restore func(), err error) {
	androidDir := filepath.Join(appDir, "android")
	info, statErr := os.Stat(androidDir)
	if statErr != nil || !info.IsDir() {
		return func() {}, nil
	}
	path := filepath.Join(androidDir, "gradle.properties")
	orig, readErr := os.ReadFile(path)
	created := false
	if readErr != nil {
		if !os.IsNotExist(readErr) {
			return nil, readErr
		}
		created = true
		orig = nil
	}
	next := string(orig)
	for _, kv := range gradleCaps {
		next = upsertProp(next, kv[0], kv[1])
	}
	if !strings.HasSuffix(next, "\n") {
		next += "\n"
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return nil, err
	}
	return func() {
		if created {
			_ = os.Remove(path)
			return
		}
		_ = os.WriteFile(path, orig, 0o644)
	}, nil
}

func upsertProp(text, key, value string) string {
	lines := strings.Split(text, "\n")
	prefix := key + "="
	found := false
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, prefix) || strings.HasPrefix(trim, key+" =") {
			lines[i] = prefix + value
			found = true
		}
	}
	if !found {
		line := prefix + value
		if len(lines) == 1 && lines[0] == "" {
			lines[0] = line
		} else if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = append(lines[:len(lines)-1], line, "")
		} else {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// apkLintInitScript turns off Android's release lint for every app and
// library in the build. assembleRelease otherwise runs lintVital, which
// loads the lint classpath into the Gradle JVM and exhausts metaspace on
// a multi-plugin Flutter app. The APK does not contain the lint report.
const apkLintInitScript = `gradle.projectsLoaded {
    rootProject.allprojects { project ->
        project.plugins.withId("com.android.application") {
            project.android.lint.checkReleaseBuilds = false
        }
        project.plugins.withId("com.android.library") {
            project.android.lint.checkReleaseBuilds = false
        }
    }
}
`

// installApkLintSkip writes a Gradle init script under the sandbox home
// and returns a restore that removes it. Gradle loads ~/.gradle/init.d
// automatically. The home directory is the workspace mount, two levels
// above the app directory.
//
// The control plane creates this directory. The APK command then runs as
// the sandbox user and needs to create ~/.gradle/tmp inside it. A fresh
// home has no .gradle yet, so the directory must be owned by uid 1000 or
// that mkdir fails.
func installApkLintSkip(appDir string) (restore func(), err error) {
	gradleHome := filepath.Join(filepath.Dir(filepath.Dir(appDir)), ".gradle")
	dir := filepath.Join(gradleHome, "init.d")
	tmp := filepath.Join(gradleHome, "tmp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "sandboxd-apk-lint.gradle")
	if err := os.WriteFile(path, []byte(apkLintInitScript), 0o644); err != nil {
		return nil, err
	}
	for _, p := range []string{gradleHome, dir, tmp, path} {
		if err := os.Chown(p, 1000, 1000); err != nil {
			return nil, err
		}
	}
	return func() { _ = os.Remove(path) }, nil
}
