package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tastyeffectco/sandboxd/control-plane/internal/runtime"
)

func TestPatchAppTaskTimeout(t *testing.T) {
	s := appsGitServer(t)
	w := createApp(s, `{"name":"notes"}`, "tenantA")
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created v1App
	if json.Unmarshal(w.Body.Bytes(), &created) != nil || created.ID == "" {
		t.Fatal(w.Body.String())
	}
	if created.TaskTimeoutS != 0 {
		t.Fatalf("new app timeout = %d; want 0", created.TaskTimeoutS)
	}

	w = patchApp(s, created.ID, `{"task_timeout_s":1800}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"task_timeout_s":1800`) {
		t.Fatalf("save %d %s", w.Code, w.Body.String())
	}
	w = patchApp(s, created.ID, `{"task_timeout_s":-1}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("negative timeout accepted: %d %s", w.Code, w.Body.String())
	}
	w = patchApp(s, created.ID, `{"task_timeout_s":86401}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("over-max timeout accepted: %d %s", w.Code, w.Body.String())
	}
	w = patchApp(s, created.ID, `{"task_timeout_s":0}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"task_timeout_s":0`) {
		t.Fatalf("reset %d %s", w.Code, w.Body.String())
	}
}

func TestClampProjectTimeout(t *testing.T) {
	if got := clampProjectTimeout(0); got != 0 {
		t.Errorf("unset = %d; want 0", got)
	}
	if got := clampProjectTimeout(1800); got != 1800 {
		t.Errorf("project = %d; want 1800", got)
	}
	if got := clampProjectTimeout(runtime.MaxTaskTimeoutS + 5); got != runtime.MaxTaskTimeoutS {
		t.Errorf("over max = %d; want %d", got, runtime.MaxTaskTimeoutS)
	}
}
