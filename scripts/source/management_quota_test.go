package main

import (
	"os"
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
