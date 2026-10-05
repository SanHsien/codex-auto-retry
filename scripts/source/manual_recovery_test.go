package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCancelKeepsTaskAsRestartableCancelledEntry(t *testing.T) {
	d := newTestDaemon(t, isolatedConfig(filepath.Join(t.TempDir(), ".codex")), successfulRunner())
	threadID := "019f9d5d-9c82-75b1-b7c0-20a658af0431"
	now := time.Date(2026, 10, 5, 7, 0, 0, 0, time.UTC)
	d.state.Threads[threadID] = ThreadState{Pending: &PendingRetry{
		EventKey: "event", FailedTurnID: "failed-turn", FailedAt: now.Add(-time.Minute),
		Class: classServer, DueAt: now.Add(time.Minute), CodexHome: "C:\\home", RolloutPath: "C:\\home\\r.jsonl",
		Attempt: 3, MaxAttempts: 15, ConsecutiveRetry: 2, MaxConsecutive: 5,
	}}
	d.applyControlCommandLocked(ControlCommand{Version: currentControlVersion, Action: commandCancelRetry, ThreadID: threadID, CreatedAt: now}, now)
	thread := d.state.Threads[threadID]
	if thread.Pending != nil || thread.Stopped == nil || thread.Stopped.Reason != stopReasonUserCancelled ||
		thread.Stopped.FailedTurnID != "failed-turn" || thread.Stopped.RolloutPath != "C:\\home\\r.jsonl" ||
		thread.Stopped.Attempts != 2 || thread.Stopped.MaxAttempts != 15 {
		t.Fatalf("cancelled retry was not kept as a restartable entry: %+v", thread.Stopped)
	}
	if !stoppedRetryIsVisible(thread.Stopped, now.Add(3*time.Hour)) {
		t.Fatal("a cancelled task disappeared after a few hours")
	}
	if stoppedRetryIsVisible(thread.Stopped, now.Add(25*time.Hour)) {
		t.Fatal("a cancelled task stayed visible beyond its window")
	}
	rows := managedRetries(d.state, now.Add(time.Hour), languageChinese)
	if len(rows) != 1 || rows[0].State != "stopped" || !rows[0].CanRestart || rows[0].StopReason != stopReasonUserCancelled {
		t.Fatalf("cancelled task is not listed for restart: %+v", rows)
	}
	if retryLimitStoppedCount(rows) != 0 {
		t.Fatal("a user cancellation was counted as a retry-limit stop")
	}
	d.applyControlCommandLocked(ControlCommand{Version: currentControlVersion, Action: commandRestartRetry, ThreadID: threadID, CreatedAt: now}, now.Add(time.Hour))
	thread = d.state.Threads[threadID]
	if thread.Stopped != nil || thread.Pending == nil || thread.Pending.FailedTurnID != "failed-turn" || thread.Pending.Attempt != 1 {
		t.Fatalf("cancelled task could not be restarted: %+v", thread)
	}
}

func TestRescanCommandNeedsNoThreadButOthersDo(t *testing.T) {
	directory := t.TempDir()
	now := time.Now().UTC()
	if _, err := queueControlCommand(directory, commandRescanInterrupted, "", now); err != nil {
		t.Fatalf("rescan without a thread was rejected: %v", err)
	}
	if _, err := queueControlCommand(directory, commandCancelRetry, "", now); err == nil {
		t.Fatal("a thread command without a thread id was accepted")
	}
}

func writeRolloutLines(t *testing.T, path string, lines ...[]byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var data []byte
	for _, line := range lines {
		data = append(data, line...)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func successfulCompletionLine(t *testing.T, timestamp, turnID string) []byte {
	t.Helper()
	line, err := json.Marshal(map[string]any{
		"timestamp": timestamp,
		"type":      "event_msg",
		"payload":   map[string]any{"type": "task_complete", "turn_id": turnID, "last_agent_message": "done"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return append(line, '\n')
}

func TestRescanFindsOnlyTasksWhoseLastTurnFailed(t *testing.T) {
	codexHome := filepath.Join(t.TempDir(), ".codex")
	sessions := filepath.Join(codexHome, "sessions", "2026", "10", "05")
	d := newTestDaemon(t, isolatedConfig(codexHome), successfulRunner())
	now := time.Now().UTC()
	at := func(offset time.Duration) string { return now.Add(offset).Format(time.RFC3339Nano) }
	rollout := func(id string) string { return filepath.Join(sessions, "rollout-2026-10-05T10-00-00-"+id+".jsonl") }

	interrupted := "019f9d5d-9c82-75b1-b7c0-20a658af0441"
	continued := "019f9d5d-9c82-75b1-b7c0-20a658af0442"
	succeeded := "019f9d5d-9c82-75b1-b7c0-20a658af0443"
	stale := "019f9d5d-9c82-75b1-b7c0-20a658af0444"
	pending := "019f9d5d-9c82-75b1-b7c0-20a658af0445"
	nonRetryable := "019f9d5d-9c82-75b1-b7c0-20a658af0446"

	writeRolloutLines(t, rollout(interrupted),
		makeEventLine(t, at(-10*time.Minute), "task_started", "turn-a", nil),
		makeEventLine(t, at(-9*time.Minute), "task_complete", "turn-a", "HTTP 503 Service Unavailable"))
	writeRolloutLines(t, rollout(continued),
		makeEventLine(t, at(-10*time.Minute), "task_complete", "turn-b", "HTTP 503 Service Unavailable"),
		makeEventLine(t, at(-5*time.Minute), "task_started", "turn-c", nil))
	writeRolloutLines(t, rollout(succeeded),
		makeEventLine(t, at(-10*time.Minute), "task_complete", "turn-d", "HTTP 503 Service Unavailable"),
		makeEventLine(t, at(-6*time.Minute), "task_started", "turn-e", nil),
		successfulCompletionLine(t, at(-5*time.Minute), "turn-e"))
	writeRolloutLines(t, rollout(stale),
		makeEventLine(t, at(-30*time.Hour), "task_complete", "turn-f", "HTTP 503 Service Unavailable"))
	old := now.Add(-30 * time.Hour)
	if err := os.Chtimes(rollout(stale), old, old); err != nil {
		t.Fatal(err)
	}
	writeRolloutLines(t, rollout(pending),
		makeEventLine(t, at(-10*time.Minute), "task_complete", "turn-g", "HTTP 503 Service Unavailable"))
	writeRolloutLines(t, rollout(nonRetryable),
		makeEventLine(t, at(-10*time.Minute), "task_complete", "turn-h", "context length exceeded: maximum context length"))
	d.state.Threads[pending] = ThreadState{Pending: &PendingRetry{EventKey: "kept", FailedTurnID: "turn-g", Class: classServer, DueAt: now, Attempt: 2}}

	found := d.rescanInterruptedLocked(now)
	if found != 1 {
		t.Fatalf("expected exactly one interrupted task, found %d: %+v", found, d.state.Threads)
	}
	thread := d.state.Threads[interrupted]
	if thread.Stopped == nil || thread.Stopped.Reason != stopReasonInterruptedDetected ||
		thread.Stopped.FailedTurnID != "turn-a" || thread.Stopped.CodexHome != codexHome ||
		thread.Stopped.Class == "" || thread.Pending != nil {
		t.Fatalf("interrupted task was not listed for review: %+v", thread)
	}
	for _, id := range []string{continued, succeeded, stale, nonRetryable} {
		if d.state.Threads[id].Stopped != nil || d.state.Threads[id].Pending != nil {
			t.Fatalf("task %s was wrongly reported as interrupted: %+v", id, d.state.Threads[id])
		}
	}
	if kept := d.state.Threads[pending].Pending; kept == nil || kept.Attempt != 2 || d.state.Threads[pending].Stopped != nil {
		t.Fatalf("an already queued task was changed: %+v", d.state.Threads[pending])
	}
	if again := d.rescanInterruptedLocked(now.Add(time.Minute)); again != 0 {
		t.Fatalf("a second rescan listed the same task again: %d", again)
	}
	d.applyControlCommandLocked(ControlCommand{Version: currentControlVersion, Action: commandRestartRetry, ThreadID: interrupted, CreatedAt: now}, now)
	if restarted := d.state.Threads[interrupted]; restarted.Pending == nil || restarted.Pending.FailedTurnID != "turn-a" {
		t.Fatalf("detected task could not be restarted: %+v", restarted)
	}
}

func TestRescanPrefersTheKnownStoreSkipsArchivedAndOldFailures(t *testing.T) {
	base := t.TempDir()
	primary := filepath.Join(base, "primary", ".codex")
	mirror := filepath.Join(base, "mirror", ".codex")
	config := isolatedConfig(primary)
	config.SessionRoots = []string{primary, mirror}
	d := newTestDaemon(t, config, successfulRunner())
	now := time.Now().UTC()
	at := func(offset time.Duration) string { return now.Add(offset).Format(time.RFC3339Nano) }
	name := func(id string) string { return "rollout-2026-10-05T10-00-00-" + id + ".jsonl" }

	mirrored := "019f9d5d-9c82-75b1-b7c0-20a658af0451"
	archived := "019f9d5d-9c82-75b1-b7c0-20a658af0452"
	oldFailure := "019f9d5d-9c82-75b1-b7c0-20a658af0453"
	failed := makeEventLine(t, at(-10*time.Minute), "task_complete", "turn-m", "HTTP 503 Service Unavailable")

	primaryPath := filepath.Join(primary, "sessions", "2026", "10", "05", name(mirrored))
	mirrorPath := filepath.Join(mirror, "sessions", "2026", "10", "05", name(mirrored))
	writeRolloutLines(t, primaryPath, failed)
	writeRolloutLines(t, mirrorPath, failed)
	earlier := now.Add(-time.Hour)
	if err := os.Chtimes(primaryPath, earlier, earlier); err != nil {
		t.Fatal(err)
	}
	// The real daemon has scanned both stores (so both are known files) before
	// a rescan; the recovery transport serves CODEX_HOME.
	t.Setenv("CODEX_HOME", primary)
	if _, err := scanSessions(discoverSessionRoots(config), &d.state, now, false); err != nil {
		t.Fatal(err)
	}

	writeRolloutLines(t, filepath.Join(primary, "archived_sessions", name(archived)),
		makeEventLine(t, at(-10*time.Minute), "task_complete", "turn-x", "HTTP 503 Service Unavailable"))
	writeRolloutLines(t, filepath.Join(primary, "sessions", "2026", "10", "05", name(oldFailure)),
		makeEventLine(t, at(-30*time.Hour), "task_complete", "turn-o", "HTTP 503 Service Unavailable"))

	if found := d.rescanInterruptedLocked(now); found != 1 {
		t.Fatalf("expected only the mirrored task, found %d: %+v", found, d.state.Threads)
	}
	if stopped := d.state.Threads[mirrored].Stopped; stopped == nil || stopped.CodexHome != primary ||
		!strings.EqualFold(stopped.RolloutPath, filepath.Clean(primaryPath)) {
		t.Fatalf("rescan did not keep the store the watchdog already follows: %+v", stopped)
	}
	for _, id := range []string{archived, oldFailure} {
		if d.state.Threads[id].Stopped != nil {
			t.Fatalf("task %s should not be listed: %+v", id, d.state.Threads[id].Stopped)
		}
	}
}

func TestRepeatedRescanCommandsRunOncePerTick(t *testing.T) {
	d := newTestDaemon(t, isolatedConfig(filepath.Join(t.TempDir(), ".codex")), successfulRunner())
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		if _, err := queueControlCommand(d.commandDir, commandRescanInterrupted, "", now.Add(time.Duration(i)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	d.refreshControlsLocked(now)
	if d.rescansThisTick != 1 {
		t.Fatalf("expected one rescan for three queued commands, got %d", d.rescansThisTick)
	}
	if entries, _ := os.ReadDir(d.commandDir); len(entries) != 0 {
		t.Fatalf("queued rescan commands were not consumed: %d left", len(entries))
	}
}

func TestTrayCountIgnoresCancelledAndInterruptedEntries(t *testing.T) {
	retries := []ManagedRetry{
		{State: "stopped", StopReason: stopReasonUserCancelled},
		{State: "stopped", StopReason: stopReasonInterruptedDetected},
		{State: "stopped", StopReason: "recovery_attempt_limit"},
		{State: "pending"},
	}
	if got := attentionStoppedCount(retries); got != 1 {
		t.Fatalf("tray would report %d stopped tasks, want 1", got)
	}
}
