package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeManagementFixture(t *testing.T, service *managementService, state RuntimeState, now time.Time) {
	t.Helper()
	if err := writeJSONAtomic(service.statePath, state); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONAtomic(service.statusPath, StatusSnapshot{
		Version: appVersion, Running: true, PID: os.Getpid(), LastScanAt: now,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestManagementExposesQuotaResetAndFailureTimes(t *testing.T) {
	service := newManagementService(t.TempDir())
	now := time.Date(2026, 10, 26, 1, 0, 0, 0, time.UTC)
	reset := now.Add(3 * time.Hour)
	waiting := "01a0b950-b558-78a2-ba57-6265343ca360"
	stopped := "01a0b950-b558-78a2-ba57-6265343ca361"
	state := newRuntimeState()
	state.Threads[waiting] = ThreadState{
		QuotaResetAt: reset,
		Pending: &PendingRetry{
			Class: classRateLimit, FailedAt: now.Add(-time.Minute), DueAt: reset.Add(quotaResetGrace),
			Attempt: 1, MaxAttempts: 15, ConsecutiveRetry: 1, MaxConsecutive: 5,
		},
	}
	state.Threads[stopped] = ThreadState{Stopped: &StoppedRetry{
		Class: classServer, FailedAt: now.Add(-10 * time.Minute), StoppedAt: now.Add(-5 * time.Minute),
		Attempts: 3, MaxAttempts: 15, ConsecutiveRetries: 1, MaxConsecutive: 5,
		Reason: "codex_thread_owner_unavailable",
	}}
	writeManagementFixture(t, service, state, now)
	snapshot, err := service.snapshot(now)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]ManagedRetry{}
	for _, retry := range snapshot.Retries {
		byID[retry.ThreadID] = retry
	}
	if got := byID[waiting].QuotaResetAt; got != reset.Format(time.RFC3339Nano) {
		t.Errorf("quota reset not exposed: %q", got)
	}
	if byID[waiting].FailedAt == "" {
		t.Errorf("failure time not exposed: %+v", byID[waiting])
	}
	if byID[stopped].QuotaResetAt != "" || byID[stopped].StoppedAt == "" || byID[stopped].FailedAt == "" {
		t.Errorf("stopped retry times wrong: %+v", byID[stopped])
	}
}

func TestManagementIgnoresPastQuotaReset(t *testing.T) {
	service := newManagementService(t.TempDir())
	now := time.Date(2026, 10, 26, 1, 0, 0, 0, time.UTC)
	threadID := "01a0b950-b558-78a2-ba57-6265343ca362"
	state := newRuntimeState()
	state.Threads[threadID] = ThreadState{
		QuotaResetAt: now.Add(-time.Hour),
		Pending:      &PendingRetry{Class: classRateLimit, DueAt: now, Attempt: 1},
	}
	writeManagementFixture(t, service, state, now)
	snapshot, err := service.snapshot(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Retries) != 1 || snapshot.Retries[0].QuotaResetAt != "" {
		t.Fatalf("an already-passed reset must not be shown: %+v", snapshot.Retries)
	}
}

func TestDismissRetryClearsOnlyStoppedThreads(t *testing.T) {
	service := newManagementService(t.TempDir())
	now := time.Date(2026, 10, 26, 1, 0, 0, 0, time.UTC)
	stoppedID := "01a0b950-b558-78a2-ba57-6265343ca363"
	pendingID := "01a0b950-b558-78a2-ba57-6265343ca364"
	state := newRuntimeState()
	state.Threads[stoppedID] = ThreadState{Stopped: &StoppedRetry{Class: classRateLimit, StoppedAt: now, Reason: "controller_unavailable"}}
	state.Threads[pendingID] = ThreadState{Pending: &PendingRetry{Class: classServer, DueAt: now.Add(time.Minute), Attempt: 1}}
	writeManagementFixture(t, service, state, now)

	snapshot, err := service.snapshot(now)
	if err != nil {
		t.Fatal(err)
	}
	for _, retry := range snapshot.Retries {
		if retry.CanDismiss != (retry.State == "stopped") {
			t.Errorf("can_dismiss wrong for %s: %+v", retry.State, retry)
		}
	}
	if _, err := service.dismissRetry(pendingID, now); err == nil {
		t.Fatal("dismissing a thread that is not stopped must be rejected")
	}
	if _, err := service.dismissRetry(stoppedID, now); err != nil {
		t.Fatal(err)
	}
	commands, _, err := loadControlCommandFiles(service.commandDir)
	if err != nil || len(commands) != 1 || commands[0].Command.Action != commandDismissRetry {
		t.Fatalf("dismiss command missing: commands=%+v err=%v", commands, err)
	}

	d := newTestDaemon(t, isolatedConfig(t.TempDir()), successfulRunner())
	d.state.Threads[stoppedID] = ThreadState{
		RecoveryAttempts: 2,
		Stopped:          &StoppedRetry{Class: classRateLimit, StoppedAt: now, Reason: "controller_unavailable"},
	}
	d.state.Threads[pendingID] = ThreadState{Pending: &PendingRetry{Class: classServer, DueAt: now.Add(time.Minute), Attempt: 1}}
	d.applyControlCommandLocked(commands[0].Command, now)
	if thread := d.state.Threads[stoppedID]; thread.Stopped != nil || thread.Pending != nil {
		t.Fatalf("dismiss did not clear the stopped record: %+v", thread)
	}
	d.applyControlCommandLocked(ControlCommand{Version: currentControlVersion, Action: commandDismissRetry, ThreadID: pendingID, CreatedAt: now}, now)
	if thread := d.state.Threads[pendingID]; thread.Pending == nil {
		t.Fatalf("dismiss must not touch a pending retry: %+v", thread)
	}
}

func TestDismissedTaskStaysDismissedAfterRescan(t *testing.T) {
	codexHome := filepath.Join(t.TempDir(), ".codex")
	sessions := filepath.Join(codexHome, "sessions", "2026", "10", "05")
	d := newTestDaemon(t, isolatedConfig(codexHome), successfulRunner())
	now := time.Now().UTC()
	threadID := "019f9d5d-9c82-75b1-b7c0-20a658af0451"
	writeRolloutLines(t, filepath.Join(sessions, "rollout-2026-10-05T10-00-00-"+threadID+".jsonl"),
		makeEventLine(t, now.Add(-10*time.Minute).Format(time.RFC3339Nano), "task_started", "turn-a", nil),
		makeEventLine(t, now.Add(-9*time.Minute).Format(time.RFC3339Nano), "task_complete", "turn-a", "HTTP 503 Service Unavailable"))
	if found := d.rescanInterruptedLocked(now); found != 1 {
		t.Fatalf("setup: expected the task to be listed, found %d", found)
	}
	d.applyControlCommandLocked(ControlCommand{Version: currentControlVersion, Action: commandDismissRetry, ThreadID: threadID, CreatedAt: now}, now)
	if d.state.Threads[threadID].Stopped != nil {
		t.Fatal("setup: dismiss did not clear the entry")
	}
	if found := d.rescanInterruptedLocked(now.Add(time.Minute)); found != 0 {
		t.Fatalf("a dismissed task came back on the next rescan: %d", found)
	}
}

func TestDismissIsRefusedForGoalLimitStops(t *testing.T) {
	service := newManagementService(t.TempDir())
	now := time.Date(2026, 10, 26, 1, 0, 0, 0, time.UTC)
	threadID := "01a0b950-b558-78a2-ba57-6265343ca365"
	state := newRuntimeState()
	state.Threads[threadID] = ThreadState{
		GoalHeld: true,
		GoalStop: &GoalStopRequest{EventKey: "k", Reason: goalEmptyResponseStopReason, RequestedAt: now, DueAt: now},
		Stopped:  &StoppedRetry{Class: classEmptyResponse, StoppedAt: now, Reason: goalEmptyResponseStopReason},
	}
	writeManagementFixture(t, service, state, now)
	snapshot, err := service.snapshot(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Retries) != 1 || snapshot.Retries[0].CanDismiss {
		t.Fatalf("a goal-limit stop must not offer dismiss: %+v", snapshot.Retries)
	}
	if _, err := service.dismissRetry(threadID, now); err == nil {
		t.Fatal("dismiss of a goal-limit stop was queued")
	}
	d := newTestDaemon(t, isolatedConfig(t.TempDir()), successfulRunner())
	d.state.Threads[threadID] = state.Threads[threadID]
	d.applyControlCommandLocked(ControlCommand{Version: currentControlVersion, Action: commandDismissRetry, ThreadID: threadID, CreatedAt: now}, now)
	thread := d.state.Threads[threadID]
	if thread.Stopped == nil || thread.GoalStop == nil {
		t.Fatalf("daemon dismissed a goal-limit stop and dropped the goal block: %+v", thread)
	}
}

func TestDismissResetsCountersLikeCancel(t *testing.T) {
	d := newTestDaemon(t, isolatedConfig(t.TempDir()), successfulRunner())
	now := time.Now().UTC()
	threadID := "01a0b950-b558-78a2-ba57-6265343ca366"
	d.state.Threads[threadID] = ThreadState{
		RecoveryAttempts: 4, ConsecutiveRetries: 3, QuotaResetAt: now.Add(time.Hour),
		Stopped: &StoppedRetry{Class: classRateLimit, FailedTurnID: "turn-x", StoppedAt: now, Reason: "codex_thread_owner_unavailable"},
	}
	d.applyControlCommandLocked(ControlCommand{Version: currentControlVersion, Action: commandDismissRetry, ThreadID: threadID, CreatedAt: now}, now)
	thread := d.state.Threads[threadID]
	if thread.RecoveryAttempts != 0 || thread.ConsecutiveRetries != 0 || !thread.QuotaResetAt.IsZero() || thread.DismissedTurnID != "turn-x" {
		t.Fatalf("dismiss left stale recovery state: %+v", thread)
	}
}
