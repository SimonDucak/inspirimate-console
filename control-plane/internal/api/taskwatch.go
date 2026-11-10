package api

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/tastyeffectco/sandboxd/control-plane/internal/events"
	"github.com/tastyeffectco/sandboxd/control-plane/internal/notifymail"
	"github.com/tastyeffectco/sandboxd/control-plane/internal/runtime"
	"github.com/tastyeffectco/sandboxd/control-plane/internal/store"
)

const (
	// watchMargin is the slack the watcher keeps beyond the task's own
	// timeout. The agent deadline is not the end of the run: the build
	// check and the preview restart use their own timers and only then
	// emit `done`. Five minutes was short of a slow build, so the
	// watcher gave up and reported a missing terminal event while
	// runtimed was still finishing.
	watchMargin       = 15 * time.Minute
	taskWatchAttempts = 3
	taskWatchRetry    = 2 * time.Second
)

// watchWindowFor is how long watchTask streams events before giving up.
// It MUST outlive the runtimed task timeout (the 10m default, or the
// caller's timeout_s): a shorter window aborts the stream and marks a
// still-running task failed. Earlier this was a fixed 15m, which
// quietly capped any task whose timeout_s exceeded ~15m.
func watchWindowFor(timeoutS int) time.Duration {
	eff := runtime.DefaultTaskTimeout
	if timeoutS > 0 {
		eff = time.Duration(timeoutS) * time.Second
	}
	return eff + watchMargin
}

// failedResult builds a clean terminal result for a task that could
// not complete normally. failure_reason is always an existing model
// value (sandbox_unavailable / internal) — no new terminal semantics
// are introduced.
func failedResult(taskID, reason, msg string) *runtime.TaskResult {
	return &runtime.TaskResult{
		ID:            taskID,
		Status:        runtime.TaskFailed,
		FailureReason: reason,
		ErrorMessage:  msg,
		FilesChanged:  []string{},
	}
}

// watchTask runs in the background for the lifetime of a coding task:
// it streams runtimed's event log and persists the canonical result
// to SQLite when the terminal `done` event arrives — independent of
// whether any client ever connects to the public events stream. This
// is what makes a task's result durable past the sandbox's lifetime.
// watchTask streams a task's events under a window derived from its
// timeout (taskTimeoutS; 0 = the runtimed default). See watchWindowFor.
func (s *Server) watchTask(sandboxID, taskID string, taskTimeoutS int) {
	s.watchTaskWindow(sandboxID, taskID, watchWindowFor(taskTimeoutS))
}

// watchTaskWindow is watchTask with an explicit streaming window. Split
// out so tests can inject a short window instead of waiting minutes.
func (s *Server) watchTaskWindow(sandboxID, taskID string, window time.Duration) {
	log := s.Log.With("component", "taskwatch", "task", taskID)
	ctx, cancel := context.WithTimeout(context.Background(), window)
	defer cancel()

	rc := s.runtimeClientFor(sandboxID)
	var result *runtime.TaskResult
	var streamErr error
	opened := false
	connectFails := 0
	gone := 0
	for result == nil {
		body, err := rc.TaskEvents(ctx, taskID, 0)
		streamErr = err
		if err == nil {
			opened = true
			connectFails = 0
			_ = runtime.DecodeEvents(body, func(ev runtime.Event) bool {
				if ev.Type == runtime.EventDone {
					var tr runtime.TaskResult
					if json.Unmarshal(ev.Data, &tr) == nil && tr.Status != "" {
						result = &tr
					}
					return false
				}
				return true
			})
			body.Close()
		} else {
			connectFails++
		}
		if result != nil {
			break
		}
		// runtimed writes result.json as it emits `done`. A dropped
		// stream can miss that event even though the task finished.
		if tr, ok := s.taskResultOnDisk(sandboxID, taskID); ok {
			result = tr
			log.Info("task watcher: recovered result from disk")
			break
		}
		if ctx.Err() != nil {
			break
		}
		// Never got a socket: give up quickly, as before. A sandbox
		// that is gone will not grow a terminal event either. A
		// sandbox that is still up gets the stream back; one blip
		// used to fail the task outright.
		if !opened && connectFails >= taskWatchAttempts {
			log.Warn("task watcher: cannot reach runtimed; marking failed", "err", errText(streamErr))
			result = failedResult(taskID, "sandbox_unavailable", "task watcher could not reach runtimed")
			break
		}
		if !s.sandboxAppearsRunning(sandboxID) {
			gone++
			if gone >= taskWatchAttempts {
				log.Warn("task watcher: cannot reach runtimed; marking failed", "err", errText(streamErr))
				result = failedResult(taskID, "sandbox_unavailable", "task watcher could not reach runtimed")
				break
			}
		} else {
			gone = 0
			log.Warn("task watcher: event stream dropped; reconnecting", "err", errText(streamErr))
		}
		select {
		case <-time.After(taskWatchRetry):
		case <-ctx.Done():
		}
	}
	if result == nil {
		if tr, ok := s.taskResultOnDisk(sandboxID, taskID); ok {
			result = tr
			log.Info("task watcher: recovered result from disk")
		} else {
			log.Warn("task watcher: event stream ended without a terminal event")
			result = failedResult(taskID, "internal", "task event stream ended without a terminal event")
		}
	}
	s.finishWatchedTask(sandboxID, taskID, result)
	s.notifyTaskDone(sandboxID, taskID, result)
	if result.FailureReason == "" {
		log.Info("task watcher: result persisted", "status", result.Status)
	}
}

// taskResultOnDisk reads the canonical result runtimed persists next to
// the event log. Missing or unreadable files are not a result.
func (s *Server) taskResultOnDisk(sandboxID, taskID string) (*runtime.TaskResult, bool) {
	if s.Loopback == nil {
		return nil, false
	}
	_, mnt := s.Loopback.Paths(sandboxID)
	raw, err := os.ReadFile(filepath.Join(mnt, ".runtimed", "tasks", taskID, "result.json"))
	if err != nil {
		return nil, false
	}
	var tr runtime.TaskResult
	if json.Unmarshal(raw, &tr) != nil || tr.Status == "" {
		return nil, false
	}
	return &tr, true
}

// sandboxAppearsRunning reports whether the sandbox row is still running.
// A missing row is not running. A lookup error is treated as running so a
// brief database failure does not fail a live task.
func (s *Server) sandboxAppearsRunning(sandboxID string) bool {
	if s.Store == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	sb, err := s.Store.Get(ctx, sandboxID)
	if err != nil {
		return !errors.Is(err, store.ErrNotFound)
	}
	return sb.Status == "running"
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// notifyTaskDone emails the project's saved addresses after a live task ends.
// Restart reconciliation does not call this, so an upgrade does not resend old results.
func (s *Server) notifyTaskDone(sandboxID, taskID string, result *runtime.TaskResult) {
	if s.Mail == nil || !s.Mail.Enabled() || result == nil {
		return
	}
	if result.Status != runtime.TaskSucceeded && result.Status != runtime.TaskFailed {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		sb, err := s.Store.Get(ctx, sandboxID)
		if err != nil || !sb.AppID.Valid {
			return
		}
		app, err := s.Store.GetApp(ctx, sb.AppID.String)
		if err != nil || len(app.NotifyEmails) == 0 {
			return
		}
		prompt := ""
		if task, err := s.Store.GetTask(ctx, taskID); err == nil {
			prompt = task.Prompt
		}
		subject, body := notifymail.TaskNotice(app.Name, string(result.Status), prompt, s.consoleAppURL(app.ID))
		if err := s.Mail.Send(ctx, app.NotifyEmails, subject, body); err != nil && s.Log != nil {
			s.Log.Warn("task email failed", "task", taskID, "err", err.Error())
		}
	}()
}

func (s *Server) consoleAppURL(appID string) string {
	if s.PreviewDomain == "" && s.ConsoleHost == "" {
		return ""
	}
	scheme, defaultPort := s.previewScheme()
	host := s.ConsoleHost
	if host == "" {
		host = s.hosts().Console()
	}
	port := ""
	if s.PublicHTTPPort != "" && s.PublicHTTPPort != defaultPort {
		port = ":" + s.PublicHTTPPort
	}
	return scheme + "://" + host + port + "/#/app/" + appID
}

func (s *Server) finishWatchedTask(sandboxID, taskID string, result *runtime.TaskResult) {
	raw, _ := json.Marshal(result)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Store.FinishTask(ctx, taskID, string(result.Status), string(raw)); err != nil {
		s.Log.Warn("task watcher: FinishTask failed", "task", taskID, "err", err.Error())
	}
	s.recordTaskEvents(sandboxID, taskID, result)
}

// recordTaskEvents appends the durable timeline entries for a finished task.
// It runs in a background path (no request), so it resolves the owning tenant
// + app from the sandbox itself. Payloads carry reasons/counts only — never
// agent output or secrets.
func (s *Server) recordTaskEvents(sandboxID, taskID string, result *runtime.TaskResult) {
	if s.Events == nil {
		return
	}
	ctx := context.Background()
	ownerToken, appID := "", ""
	if sb, err := s.Store.Get(ctx, sandboxID); err == nil && sb.AppID.Valid {
		appID = sb.AppID.String
		if app, aerr := s.Store.GetApp(ctx, appID); aerr == nil {
			ownerToken = app.OwnerToken
		}
	}
	base := events.Event{OwnerToken: ownerToken, AppID: appID, SandboxID: sandboxID, TaskID: taskID}

	rec := func(typ, sev, msg string, payload map[string]any) {
		e := base
		e.Type, e.Severity, e.Message, e.Payload = typ, sev, msg, payload
		s.Events.Record(ctx, e)
	}

	// Payloads carry safe STRUCTURED flags/reasons only — never the raw
	// build/dev-server/agent output (which can echo secrets the user's app
	// printed). The full text stays in the task's result.json.
	if result.Status == runtime.TaskSucceeded {
		rec(events.TaskCompleted, events.SeverityInfo, "Task completed",
			map[string]any{"files_changed": len(result.FilesChanged), "duration_ms": result.DurationMS, "build_ok": result.BuildOK})
	} else {
		rec(events.TaskFailed, events.SeverityError, "Task failed: "+string(result.Status),
			map[string]any{"failure_reason": result.FailureReason, "has_error": result.ErrorMessage != ""})
	}
	// A real build failure (distinct from infra failures, which leave
	// BuildErrorMessage empty).
	if result.BuildErrorMessage != "" {
		rec(events.TaskBuildFailed, events.SeverityError, "Task build failed",
			map[string]any{"reason": "build_failed", "has_build_error": true})
	}
	// Preview health after the task: report only a clear signal. preview_status
	// is the structured enum (down/starting/ready/error), not raw output.
	switch {
	case result.PreviewErrorMessage != "" || result.PreviewStatusAfter == runtime.PreviewError:
		rec(events.PreviewHealthFailed, events.SeverityWarning, "Preview unhealthy after task",
			map[string]any{"preview_status": string(result.PreviewStatusAfter), "has_preview_error": result.PreviewErrorMessage != ""})
	case result.PreviewStatusAfter == runtime.PreviewReady:
		rec(events.PreviewHealthOK, events.SeverityInfo, "Preview healthy after task", nil)
	}
}

// ReconcileTasks finalizes tasks left `running` by a previous sandboxd
// run (whose watcher goroutines did not survive the restart). Run once
// at boot, before the idle reaper — which trusts the task table —
// begins ticking. Per running row:
//   - runtimed already wrote result.json -> finalize from it;
//   - else the sandbox is still running -> re-attach a watcher;
//   - else -> finalize as a clean sandbox_unavailable failure.
//
// An interrupted task therefore lands on the existing failure model
// (status=failed, failure_reason=sandbox_unavailable) — no new
// terminal state is introduced.
func (s *Server) ReconcileTasks(ctx context.Context) {
	tasks, err := s.Store.ListRunningTasks(ctx)
	if err != nil {
		s.Log.Warn("task reconcile: list running tasks failed", "err", err.Error())
		return
	}
	for _, t := range tasks {
		_, mnt := s.Loopback.Paths(t.SandboxID)
		resultPath := filepath.Join(mnt, ".runtimed", "tasks", t.TaskID, "result.json")
		if raw, rerr := os.ReadFile(resultPath); rerr == nil {
			var tr runtime.TaskResult
			if json.Unmarshal(raw, &tr) == nil && tr.Status != "" {
				s.finishWatchedTask(t.SandboxID, t.TaskID, &tr)
				s.Log.Info("task reconcile: finalized from runtimed result.json",
					"task", t.TaskID, "status", tr.Status)
				continue
			}
		}
		if sb, gerr := s.Store.Get(ctx, t.SandboxID); gerr == nil && sb.Status == "running" {
			s.Log.Info("task reconcile: sandbox running — re-attaching watcher", "task", t.TaskID)
			go s.watchTask(t.SandboxID, t.TaskID, t.TimeoutS)
			continue
		}
		s.finishWatchedTask(t.SandboxID, t.TaskID, failedResult(t.TaskID, "sandbox_unavailable",
			"task interrupted by a sandboxd restart; the sandbox was unavailable to resume or report it"))
		s.Log.Info("task reconcile: finalized interrupted task", "task", t.TaskID)
	}
}
