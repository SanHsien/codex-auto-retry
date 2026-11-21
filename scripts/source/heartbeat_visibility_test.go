package main

import (
	"context"
	"os"
	"testing"
	"time"
)

// R-20: a live worker whose scans take longer than the poll interval must not
// be reported as stopped, and its waiting tasks must stay visible.
func TestManagementTrustsHeartbeatWithinMeasuredScanInterval(t *testing.T) {
	service := newManagementService(t.TempDir())
	now := time.Now().UTC()
	state := newRuntimeState()
	state.Threads["01a1fbc7-0000-7000-8000-000000000001"] = ThreadState{Pending: &PendingRetry{
		Class: classRateLimit, FailedAt: now.Add(-time.Minute), DueAt: now.Add(20 * time.Minute),
		Attempt: 1, MaxAttempts: 15, ConsecutiveRetry: 1, MaxConsecutive: 5,
	}}
	if err := writeJSONAtomic(service.statePath, state); err != nil {
		t.Fatal(err)
	}
	// Measured on the 2026-11-21 host: scans 25-50 s apart.
	if err := writeJSONAtomic(service.statusPath, StatusSnapshot{
		Version: appVersion, Running: true, PID: os.Getpid(),
		LastScanAt: now.Add(-40 * time.Second), ScanIntervalMs: (35 * time.Second).Milliseconds(),
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.snapshot(now)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Running || snapshot.HeartbeatStale || snapshot.PendingRetries != 1 {
		t.Fatalf("slow but live worker was reported as stopped or hid its queue: running=%v stale=%v pending=%d",
			snapshot.Running, snapshot.HeartbeatStale, snapshot.PendingRetries)
	}
}

// The measured period widens the window; it must not disable stale detection.
func TestManagementFlagsHeartbeatFarBeyondMeasuredScanInterval(t *testing.T) {
	service := newManagementService(t.TempDir())
	now := time.Now().UTC()
	if err := writeJSONAtomic(service.statusPath, StatusSnapshot{
		Version: appVersion, Running: true, PID: os.Getpid(),
		LastScanAt: now.Add(-5 * time.Minute), ScanIntervalMs: (35 * time.Second).Milliseconds(),
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.snapshot(now)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Running || !snapshot.HeartbeatStale {
		t.Fatalf("hung worker was reported as healthy: running=%v stale=%v", snapshot.Running, snapshot.HeartbeatStale)
	}
}

func TestTickPublishesMeasuredScanInterval(t *testing.T) {
	d := newTestDaemon(t, isolatedConfig(t.TempDir()), successfulRunner())
	first := time.Now().UTC()
	if err := d.tick(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := d.tick(context.Background(), first.Add(40*time.Second)); err != nil {
		t.Fatal(err)
	}
	status, found, err := loadStatusSnapshot(d.statusPath)
	if err != nil || !found {
		t.Fatalf("status missing: found=%v err=%v", found, err)
	}
	got := time.Duration(status.ScanIntervalMs) * time.Millisecond
	if got < 39*time.Second || got > 41*time.Second {
		t.Fatalf("scan interval between heartbeats 40 s apart was published as %v", got)
	}
}

// last_scan_at went backwards on the host. time.Ticker delivers the time a
// tick was scheduled; after a scan overruns the poll interval, the queued
// value is already stale by the overrun. The heartbeat must use the clock.
func TestRunHeartbeatIgnoresStaleTickerValue(t *testing.T) {
	d := newTestDaemon(t, isolatedConfig(t.TempDir()), successfulRunner())
	ticks := make(chan time.Time)
	d.newTicker = func(time.Duration) (<-chan time.Time, func()) { return ticks, func() {} }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.run(ctx) }()
	sent := time.Now().UTC()
	stale := sent.Add(-30 * time.Second)
	ticks <- stale
	ticks <- stale // accepted only after the first queued tick finished
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	got := d.lastScan
	d.mu.Unlock()
	if got.Before(sent) {
		t.Fatalf("heartbeat %v is older than the scan that wrote it (started after %v)", got, sent)
	}
}

// R-21: a quota wait holds the failure on purpose, so a task that waited more
// than the display window and then stopped must still be listed.
func TestManagementShowsQuotaWaitStoppedAfterLongWait(t *testing.T) {
	service := newManagementService(t.TempDir())
	// Times from the 2026-11-21 host run (task 01a1fbc7).
	failed := time.Date(2026, 11, 21, 1, 46, 7, 0, time.UTC)
	reset := time.Date(2026, 11, 21, 3, 16, 0, 0, time.UTC)
	stoppedAt := time.Date(2026, 11, 21, 3, 16, 41, 0, time.UTC)
	now := time.Date(2026, 11, 21, 3, 25, 10, 0, time.UTC)
	threadID := "01a1fbc7-0000-7000-8000-000000000002"
	transportBlocked := true
	state := newRuntimeState()
	state.Threads[threadID] = ThreadState{QuotaResetAt: reset, Stopped: &StoppedRetry{
		Class: classRateLimit, FailedAt: failed, StoppedAt: stoppedAt, Reason: "shared_app_server_disabled",
		MaxAttempts: 15, MaxConsecutive: 5, TransportBlocked: &transportBlocked,
	}}
	writeManagementFixture(t, service, state, now)
	snapshot, err := service.snapshot(now)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.StoppedRetries != 1 || len(snapshot.Retries) != 1 {
		t.Fatalf("task stopped after a long quota wait was hidden: stopped=%d retries=%+v", snapshot.StoppedRetries, snapshot.Retries)
	}
	if retry := snapshot.Retries[0]; !retry.CanRestart || !retry.CanDismiss {
		t.Fatalf("listed task cannot be restarted or dismissed: %+v", retry)
	}
}

// The failure-age guard still hides an old failure that was only observed
// and stopped now, without any quota wait in between.
func TestManagementHidesOldFailureStoppedNowWithoutQuotaWait(t *testing.T) {
	service := newManagementService(t.TempDir())
	now := time.Date(2026, 11, 21, 3, 25, 10, 0, time.UTC)
	state := newRuntimeState()
	state.Threads["01a1fbc7-0000-7000-8000-000000000003"] = ThreadState{Stopped: &StoppedRetry{
		Class: classServer, FailedAt: now.Add(-2 * time.Hour), StoppedAt: now, Reason: "recovery_attempt_limit",
		MaxAttempts: 15, MaxConsecutive: 5,
	}}
	writeManagementFixture(t, service, state, now)
	snapshot, err := service.snapshot(now)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.StoppedRetries != 0 {
		t.Fatalf("old failure stopped without a quota wait was listed: %+v", snapshot.Retries)
	}
}
