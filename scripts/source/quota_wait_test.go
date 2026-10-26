package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func ordinalDay(day int) string {
	suffix := "th"
	if day%100 < 11 || day%100 > 13 {
		switch day % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", day, suffix)
}

func usageLimitText(resetAt time.Time) string {
	local := resetAt.In(time.Local)
	return fmt.Sprintf("You've hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), "+
		"visit https://chatgpt.com/settings/usage to purchase more credits or try again at %s %s, %d %s.",
		local.Format("Jan"), ordinalDay(local.Day()), local.Year(), local.Format("3:04 PM"))
}

func TestUsageLimitResetAtParsesCodexMessages(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*3600)
	now := time.Date(2026, 10, 26, 1, 0, 0, 0, loc)
	cases := []struct {
		name string
		text string
		want time.Time
		ok   bool
	}{
		{"dated", "... or try again at Oct 9th, 2026 3:41 AM.", time.Date(2026, 10, 9, 3, 41, 0, 0, loc), true},
		{"dated without year", "try again at Oct 27th 11:05 PM.", time.Date(2026, 10, 27, 23, 5, 0, 0, loc), true},
		{"clock later today", "try again at 5:39 AM.", time.Date(2026, 10, 26, 5, 39, 0, 0, loc), true},
		{"clock already passed rolls to tomorrow", "try again at 12:30 AM.", time.Date(2026, 10, 27, 0, 30, 0, 0, loc), true},
		{"case and spacing", "Try Again At   2:48 pm", time.Date(2026, 10, 26, 14, 48, 0, 0, loc), true},
		{"no time", "exceeded retry limit, last status: 429 Too Many Requests", time.Time{}, false},
		{"try again later is not a time", "please try again later", time.Time{}, false},
		{"impossible clock", "try again at 25:99 AM", time.Time{}, false},
	}
	for _, tc := range cases {
		got, ok := usageLimitResetAt(tc.text, now, loc)
		if ok != tc.ok || (ok && !got.Equal(tc.want)) {
			t.Errorf("%s: got %v ok=%v, want %v ok=%v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestUsageLimitMessagesClassifyAsRateLimit(t *testing.T) {
	for _, text := range []string{
		"You've hit your usage limit. Upgrade to Pro or try again at 5:39 AM.",
		"Quota exceeded. Check your plan and billing details.",
		"usage_limit_exceeded",
	} {
		decision := classifyFailure(text, defaultConfig())
		if !decision.Retry || decision.Class != classRateLimit {
			t.Errorf("%q classified as %+v, want a retryable rate_limit", text, decision)
		}
	}
}

func TestUsageLimitRetryWaitsUntilReset(t *testing.T) {
	threadID := "01a0b950-b558-78a2-ba57-6265343ca360"
	now := time.Now().UTC().Truncate(time.Second)
	reset := now.Add(3 * time.Hour).Truncate(time.Minute) // the message only has minute precision
	d := newTestDaemon(t, isolatedConfig(t.TempDir()), successfulRunner())
	d.handleEventLocked(scannedEvent{ThreadID: threadID, Event: RelevantEvent{
		Kind: "task_complete", TurnID: "limited-turn", Timestamp: now,
		ErrorText: usageLimitText(reset), FinalKnown: true,
	}}, now)
	thread := daemonThreadSnapshot(d, threadID)
	if thread.Pending == nil || thread.Pending.Class != classRateLimit {
		t.Fatalf("usage limit was not queued as a rate_limit retry: %+v", thread)
	}
	if thread.Pending.DueAt.Before(reset) {
		t.Fatalf("retry is due at %v, before the quota resets at %v", thread.Pending.DueAt, reset)
	}
	if thread.Pending.DueAt.After(reset.Add(5 * time.Minute)) {
		t.Fatalf("retry is due long after the reset: %v vs %v", thread.Pending.DueAt, reset)
	}
	if jobs := d.dispatchDueLocked(now.Add(2 * time.Hour)); len(jobs) != 0 {
		t.Fatalf("retry fired before the quota reset: %+v", jobs)
	}
	// The 30 minute automatic recovery window must start at the reset, not at
	// the failure, otherwise the retry is abandoned before it is due.
	after := reset.Add(2 * time.Minute)
	jobs := d.dispatchDueLocked(after)
	if len(jobs) != 1 {
		t.Fatalf("retry did not fire after the reset: jobs=%d thread=%+v", len(jobs), daemonThreadSnapshot(d, threadID))
	}
	if thread := daemonThreadSnapshot(d, threadID); thread.Stopped != nil {
		t.Fatalf("retry stopped instead of dispatching: %+v", thread.Stopped)
	}
}

func TestUsageLimitWithoutResetTimeKeepsNormalSchedule(t *testing.T) {
	threadID := "01a0b950-b558-78a2-ba57-6265343ca361"
	now := time.Now().UTC().Truncate(time.Second)
	d := newTestDaemon(t, isolatedConfig(t.TempDir()), successfulRunner())
	d.handleEventLocked(scannedEvent{ThreadID: threadID, Event: RelevantEvent{
		Kind: "task_complete", TurnID: "limited-turn", Timestamp: now,
		ErrorText: "exceeded retry limit, last status: 429 Too Many Requests", FinalKnown: true,
	}}, now)
	thread := daemonThreadSnapshot(d, threadID)
	if thread.Pending == nil || thread.Pending.DueAt.After(now.Add(time.Minute)) {
		t.Fatalf("a 429 without a reset time must keep the short schedule: %+v", thread.Pending)
	}
}

func quotaPending(now time.Time, class FailureClass, failures int) *PendingRetry {
	return &PendingRetry{
		EventKey: "event", FailedTurnID: "failed", Class: class, DueAt: now,
		Attempt: 1, MaxAttempts: 15, ConsecutiveRetry: 1, MaxConsecutive: 5,
		DispatchFailures: failures,
	}
}

func runOneDispatch(t *testing.T, d *daemon, now time.Time) {
	t.Helper()
	jobs := d.dispatchDueLocked(now)
	if len(jobs) != 1 {
		t.Fatalf("expected one job, got %d", len(jobs))
	}
	d.wg.Add(1)
	go d.runJob(context.Background(), jobs[0])
	d.waitForJobs()
}

func TestQuotaRetryOutlastsControllerFailureLimit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		runner *fakeResumeRunner
	}{
		{"controller error", &fakeResumeRunner{err: errors.New("controller unavailable")}},
		{"thread owner missing", &fakeResumeRunner{result: DispatchResult{Outcome: outcomeRetryLater, Reason: "codex_thread_owner_unavailable"}}},
		{"channel unavailable", &fakeResumeRunner{result: DispatchResult{Outcome: outcomeRetryLater, Reason: "codex_background_channel_unavailable"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			threadID := "01a1743a-55b4-7f60-aa57-3aee25b0590f"
			now := time.Now().UTC()
			cfg := isolatedConfig(t.TempDir())
			d := newTestDaemon(t, cfg, tc.runner)
			d.state.Threads[threadID] = ThreadState{
				RecoveryStartedAt: now,
				Pending:           quotaPending(now, classRateLimit, cfg.ControllerFailureLimit-1),
			}
			runOneDispatch(t, d, now)
			thread := daemonThreadSnapshot(d, threadID)
			if thread.Stopped != nil || thread.Pending == nil || thread.Awaiting != nil {
				t.Fatalf("quota retry gave up on a temporary controller failure: %+v", thread)
			}
			if thread.Pending.DispatchFailures != cfg.ControllerFailureLimit {
				t.Fatalf("failure was not counted for back-off: %+v", thread.Pending)
			}
			if !thread.Pending.DueAt.After(now) {
				t.Fatalf("retry was not delayed: %+v", thread.Pending)
			}
		})
	}
}

func TestQuotaRetryStillStopsAtRecoveryWindow(t *testing.T) {
	threadID := "01a1743a-55b4-7f60-aa57-3aee25b0590f"
	now := time.Now().UTC()
	d := newTestDaemon(t, isolatedConfig(t.TempDir()), successfulRunner())
	d.state.Threads[threadID] = ThreadState{
		RecoveryStartedAt: now.Add(-maxAutomaticRecoveryDuration - time.Minute),
		Pending:           quotaPending(now, classRateLimit, 9),
	}
	if jobs := d.dispatchDueLocked(now); len(jobs) != 0 {
		t.Fatalf("expired quota retry was dispatched: %+v", jobs)
	}
	if thread := daemonThreadSnapshot(d, threadID); thread.Stopped == nil || thread.Stopped.Reason != "recovery_time_limit" {
		t.Fatalf("quota retry was not bounded by the recovery window: %+v", thread)
	}
}

type deadlineRunner struct {
	*fakeResumeRunner
	mu        sync.Mutex
	calls     int
	remaining []time.Duration
	err       error
}

func (r *deadlineRunner) RetryThreadStatus(ctx context.Context, _, _ string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if deadline, ok := ctx.Deadline(); ok {
		r.remaining = append(r.remaining, time.Until(deadline))
	}
	return "", r.err
}

func awaitingForProbe(now time.Time, class FailureClass, failures int, startedAgo time.Duration) *AwaitingRetry {
	return &AwaitingRetry{
		EventKey: "event", FailedTurnID: "failed", RetryTurnID: "retry-turn", Class: class,
		Attempt: 1, MaxAttempts: 15, ConsecutiveRetry: 1, MaxConsecutive: 5,
		DispatchFailures: failures,
		StartedAt:        now.Add(-startedAgo), DispatchStartedAt: now.Add(-startedAgo),
		StartDeadline: now.Add(-startedAgo),
	}
}

func TestLifecycleProbeBudgetCoversDesktopTransportCheck(t *testing.T) {
	threadID := "01a0b950-b558-78a2-ba57-6265343ca362"
	runner := &deadlineRunner{fakeResumeRunner: successfulRunner(), err: errors.New("probe failed")}
	d := newTestDaemon(t, isolatedConfig(t.TempDir()), runner)
	now := time.Now().UTC()
	d.state.Threads[threadID] = ThreadState{Awaiting: awaitingForProbe(now, classRateLimit, 0, time.Minute)}
	d.reconcileAwaitingLifecycle(context.Background(), now)
	if len(runner.remaining) != 1 {
		t.Fatalf("probe did not run once: %d", len(runner.remaining))
	}
	// The desktop transport check shells out to PowerShell with its own
	// timeout. A probe budget shorter than that timeout reports a slow machine
	// as controller_unavailable.
	if runner.remaining[0] <= desktopTransportCheckTimeout {
		t.Fatalf("probe budget %v does not exceed the transport check timeout %v", runner.remaining[0], desktopTransportCheckTimeout)
	}
}

func TestQuotaLifecycleProbeFailuresBackOffInsteadOfStopping(t *testing.T) {
	threadID := "01a0b950-b558-78a2-ba57-6265343ca363"
	cfg := isolatedConfig(t.TempDir())
	runner := &deadlineRunner{fakeResumeRunner: successfulRunner(), err: errors.New("probe failed")}
	d := newTestDaemon(t, cfg, runner)
	now := time.Now().UTC()
	d.state.Threads[threadID] = ThreadState{Awaiting: awaitingForProbe(now, classRateLimit, cfg.ControllerFailureLimit-1, time.Minute)}

	d.reconcileAwaitingLifecycle(context.Background(), now)
	thread := daemonThreadSnapshot(d, threadID)
	if thread.Stopped != nil || thread.Awaiting == nil {
		t.Fatalf("quota retry stopped on a probe failure: %+v", thread)
	}
	// An immediate second tick must not probe again: failures back off.
	d.reconcileAwaitingLifecycle(context.Background(), now.Add(6*time.Second))
	if runner.calls != 1 {
		t.Fatalf("probe was repeated without back-off: calls=%d", runner.calls)
	}
	// Eventually it probes again, still without stopping.
	d.reconcileAwaitingLifecycle(context.Background(), now.Add(10*time.Minute))
	if runner.calls != 2 {
		t.Fatalf("probe never resumed after back-off: calls=%d", runner.calls)
	}
	if thread := daemonThreadSnapshot(d, threadID); thread.Stopped != nil || thread.Awaiting == nil {
		t.Fatalf("quota retry stopped after repeated probe failures: %+v", thread)
	}
}

func TestQuotaLifecycleProbeGivesUpAfterRecoveryWindow(t *testing.T) {
	threadID := "01a0b950-b558-78a2-ba57-6265343ca364"
	runner := &deadlineRunner{fakeResumeRunner: successfulRunner(), err: errors.New("probe failed")}
	d := newTestDaemon(t, isolatedConfig(t.TempDir()), runner)
	now := time.Now().UTC()
	d.state.Threads[threadID] = ThreadState{Awaiting: awaitingForProbe(now, classRateLimit, 50, maxAutomaticRecoveryDuration+time.Minute)}
	d.reconcileAwaitingLifecycle(context.Background(), now)
	if thread := daemonThreadSnapshot(d, threadID); thread.Stopped == nil || thread.Awaiting != nil {
		t.Fatalf("probe failures are not bounded by the recovery window: %+v", thread)
	}
}

func TestControllerDeadlineIsReportedAsTimeout(t *testing.T) {
	for _, err := range []error{
		context.DeadlineExceeded,
		fmt.Errorf("probe: %w", context.DeadlineExceeded),
		wrapControllerCommandError(expiredContext(), errors.New("signal: killed")),
	} {
		if reason := controllerFailureReason(DispatchResult{}, err); reason != "controller_timeout" {
			t.Errorf("%v reported as %q, want controller_timeout", err, reason)
		}
	}
	if reason := controllerFailureReason(DispatchResult{}, wrapControllerCommandError(context.Background(), errors.New("exit status 1"))); strings.Contains(reason, "timeout") {
		t.Errorf("a plain command failure must not be reported as a timeout: %q", reason)
	}
}

func expiredContext() context.Context {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	cancel()
	return ctx
}

func TestQuotaWindowSurvivesSecondFailureWithoutResetTime(t *testing.T) {
	threadID := "01a0b950-b558-78a2-ba57-6265343ca370"
	now := time.Now().UTC().Truncate(time.Second)
	reset := now.Add(3 * time.Hour)
	d := newTestDaemon(t, isolatedConfig(t.TempDir()), successfulRunner())
	d.handleEventLocked(scannedEvent{ThreadID: threadID, Event: RelevantEvent{
		Kind: "task_complete", TurnID: "t1", Timestamp: now, ErrorText: usageLimitText(reset), FinalKnown: true,
	}}, now)
	after := reset.Add(time.Minute)
	jobs := d.dispatchDueLocked(after)
	if len(jobs) != 1 {
		t.Fatalf("first retry did not fire: %+v", daemonThreadSnapshot(d, threadID))
	}
	// The retried turn fails again with a plain 429 (no reset time).
	d.mu.Lock()
	thread := d.state.Threads[threadID]
	thread.Awaiting = nil
	d.state.Threads[threadID] = thread
	delete(d.active, threadID)
	d.mu.Unlock()
	failedAt := after.Add(10 * time.Second)
	d.handleEventLocked(scannedEvent{ThreadID: threadID, Event: RelevantEvent{
		Kind: "task_complete", TurnID: "t2", Timestamp: failedAt,
		ErrorText: "exceeded retry limit, last status: 429 Too Many Requests", FinalKnown: true,
	}}, failedAt)
	thread = daemonThreadSnapshot(d, threadID)
	if thread.Stopped != nil || thread.Pending == nil {
		t.Fatalf("a second failure right after the reset ended the recovery: %+v", thread)
	}
}

func TestBareResetTimeIsRelativeToTheEventNotToProcessingTime(t *testing.T) {
	threadID := "01a0b950-b558-78a2-ba57-6265343ca371"
	loc := time.Local
	eventAt := time.Date(2026, 10, 26, 3, 0, 0, 0, loc)
	processedAt := time.Date(2026, 10, 26, 6, 0, 0, 0, loc)
	d := newTestDaemon(t, isolatedConfig(t.TempDir()), successfulRunner())
	d.handleEventLocked(scannedEvent{ThreadID: threadID, Event: RelevantEvent{
		Kind: "task_complete", TurnID: "t1", Timestamp: eventAt,
		ErrorText: "You've hit your usage limit. Try again at 5:39 AM.", FinalKnown: true,
	}}, processedAt)
	thread := daemonThreadSnapshot(d, threadID)
	if thread.Pending == nil || thread.Pending.DueAt.After(processedAt.Add(time.Hour)) {
		t.Fatalf("a reset that passed while the daemon was down must not wait another day: %+v", thread.Pending)
	}
}

func TestQuotaDispatchGateKeepsRateLimitQueuedWhileControllerRecovers(t *testing.T) {
	for _, state := range []string{"codex_thread_owner_unavailable", "controller_timeout", "codex_background_channel_unavailable"} {
		t.Run(state, func(t *testing.T) {
			threadID := "01a1743a-55b4-7f60-aa57-3aee25b0590f"
			now := time.Now().UTC()
			cfg := isolatedConfig(t.TempDir())
			cfg.SharedAppServerEnabled = false
			d := newTestDaemon(t, cfg, successfulRunner())
			d.controllerState = state
			d.state.Threads[threadID] = ThreadState{RecoveryStartedAt: now, Pending: quotaPending(now, classRateLimit, 0)}
			if jobs := d.dispatchDueLocked(now); len(jobs) != 0 {
				t.Fatalf("dispatched while the controller was down: %+v", jobs)
			}
			if thread := daemonThreadSnapshot(d, threadID); thread.Stopped != nil || thread.Pending == nil {
				t.Fatalf("transient controller state stopped a quota retry at the gate: %+v", thread)
			}
		})
	}
	t.Run("other classes still stop", func(t *testing.T) {
		threadID := "01a1743a-55b4-7f60-aa57-3aee25b0590e"
		now := time.Now().UTC()
		cfg := isolatedConfig(t.TempDir())
		cfg.SharedAppServerEnabled = false
		d := newTestDaemon(t, cfg, successfulRunner())
		d.controllerState = "codex_thread_owner_unavailable"
		d.state.Threads[threadID] = ThreadState{Pending: quotaPending(now, classServer, 0)}
		d.dispatchDueLocked(now)
		if thread := daemonThreadSnapshot(d, threadID); thread.Stopped == nil {
			t.Fatalf("non-quota retry should keep the old gate behavior: %+v", thread)
		}
	})
	for _, state := range []string{"shared_app_server_disabled", "codex_not_running", "codex_restart_required"} {
		t.Run("needs the user: "+state, func(t *testing.T) {
			threadID := "01a1743a-55b4-7f60-aa57-3aee25b0590d"
			now := time.Now().UTC()
			cfg := isolatedConfig(t.TempDir())
			cfg.SharedAppServerEnabled = false
			d := newTestDaemon(t, cfg, successfulRunner())
			d.controllerState = state
			d.state.Threads[threadID] = ThreadState{Pending: quotaPending(now, classRateLimit, 0)}
			d.dispatchDueLocked(now)
			if thread := daemonThreadSnapshot(d, threadID); thread.Stopped == nil {
				t.Fatalf("a state that needs the user must still stop the retry: %+v", thread)
			}
		})
	}
}

func TestQuotaRetryDoesNotRepeatAmbiguousDispatch(t *testing.T) {
	threadID := "01a1743a-55b4-7f60-aa57-3aee25b0590c"
	now := time.Now().UTC()
	cfg := isolatedConfig(t.TempDir())
	d := newTestDaemon(t, cfg, successfulRunner())
	d.state.Threads[threadID] = ThreadState{Awaiting: &AwaitingRetry{
		EventKey: "event", FailedTurnID: "failed", Class: classRateLimit, Attempt: 1, MaxAttempts: 15,
		ConsecutiveRetry: 1, MaxConsecutive: 5, DispatchFailures: cfg.ControllerFailureLimit - 1,
		DispatchStartedAt: now.Add(-time.Minute), StartDeadline: now.Add(-time.Second),
	}}
	d.expireUnacknowledgedLocked(now)
	if thread := daemonThreadSnapshot(d, threadID); thread.Stopped == nil || thread.Stopped.Reason != "start_not_acknowledged" {
		t.Fatalf("an unacknowledged send may already have started a turn; it must keep the failure limit: %+v", thread)
	}
}

func TestQuotaAmbiguousControllerErrorsGetALargerButFiniteLimit(t *testing.T) {
	threadID := "01a1743a-55b4-7f60-aa57-3aee25b0590b"
	now := time.Now().UTC()
	cfg := isolatedConfig(t.TempDir())
	d := newTestDaemon(t, cfg, &fakeResumeRunner{err: errors.New("controller unavailable")})
	d.state.Threads[threadID] = ThreadState{RecoveryStartedAt: now, Pending: quotaPending(now, classRateLimit, cfg.ControllerFailureLimit*4-1)}
	runOneDispatch(t, d, now)
	if thread := daemonThreadSnapshot(d, threadID); thread.Stopped == nil {
		t.Fatalf("ambiguous controller errors must still be finite: %+v", thread)
	}
}

func TestClassifierDoesNotTreatDiskQuotaAsUsageLimit(t *testing.T) {
	decision := classifyFailure("write failed: Disk quota exceeded", defaultConfig())
	if decision.Class == classRateLimit {
		t.Fatalf("unrelated quota error classified as rate_limit: %+v", decision)
	}
}

func TestTransportReopenUsesQuotaRecoveryWindow(t *testing.T) {
	now := time.Now().UTC()
	thread := transportStoppedThread(now)
	// The failure happened hours ago; only the quota reset a minute ago starts
	// the 30 minute window.
	thread.RecoveryStartedAt = now.Add(-3 * time.Hour)
	thread.Stopped.FailedAt = now.Add(-3 * time.Hour)
	thread.LastStartedAt = now.Add(-3*time.Hour - time.Second)
	thread.QuotaResetAt = now.Add(-time.Minute)
	if !canReopenTransportRetry(thread, now) {
		t.Fatal("a quota retry stopped by a transport glitch right after its reset must be reopenable")
	}
	thread.QuotaResetAt = now.Add(-maxAutomaticRecoveryDuration - time.Minute)
	if canReopenTransportRetry(thread, now) {
		t.Fatal("the reopen window must still end 30 minutes after the reset")
	}
}

func TestTickHeartbeatIncludesProbeTime(t *testing.T) {
	cfg := isolatedConfig(t.TempDir())
	runner := &slowProbeRunner{fakeResumeRunner: successfulRunner(), delay: 150 * time.Millisecond}
	d := newTestDaemon(t, cfg, runner)
	now := time.Now().UTC()
	if err := d.tick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if got := d.lastScan.Sub(now); got < runner.delay {
		t.Fatalf("heartbeat was stamped at the tick start, before %v of probing: advanced %v", runner.delay, got)
	}
}

type slowProbeRunner struct {
	*fakeResumeRunner
	delay time.Duration
}

func (r *slowProbeRunner) ControllerState(context.Context) string {
	time.Sleep(r.delay)
	return "official_ipc_ready"
}

func TestQuotaResetDoesNotLeakIntoALaterChain(t *testing.T) {
	threadID := "01a0b950-b558-78a2-ba57-6265343ca372"
	now := time.Now().UTC().Truncate(time.Second)
	d := newTestDaemon(t, isolatedConfig(t.TempDir()), successfulRunner())
	d.handleEventLocked(scannedEvent{ThreadID: threadID, Event: RelevantEvent{
		Kind: "task_complete", TurnID: "t1", Timestamp: now,
		ErrorText: usageLimitText(now.Add(100 * time.Hour)), FinalKnown: true,
	}}, now)
	if daemonThreadSnapshot(d, threadID).QuotaResetAt.IsZero() {
		t.Fatal("setup: reset not recorded")
	}
	// A successful turn ends the chain.
	d.mu.Lock()
	d.resetRetryStateLocked(threadID, d.state.Threads[threadID])
	d.mu.Unlock()
	if !daemonThreadSnapshot(d, threadID).QuotaResetAt.IsZero() {
		t.Fatal("a finished chain kept its quota reset")
	}
	// A new unrelated failure must start from a clean window.
	later := now.Add(time.Hour)
	d.handleEventLocked(scannedEvent{ThreadID: threadID, Event: RelevantEvent{
		Kind: "task_complete", TurnID: "t2", Timestamp: later, ErrorText: "stream disconnected", FinalKnown: true,
	}}, later)
	if thread := daemonThreadSnapshot(d, threadID); !thread.QuotaResetAt.IsZero() {
		t.Fatalf("new chain inherited an old reset: %+v", thread.QuotaResetAt)
	}
}
