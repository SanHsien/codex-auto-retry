package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Manual recovery keeps tasks the user can still act on in the queue instead
// of dropping them: a cancelled retry, or an interrupted task found by an
// explicit rescan. Both wait for the user to press Restart; neither is ever
// dispatched automatically.
const (
	stopReasonUserCancelled       = "user_cancelled"
	stopReasonInterruptedDetected = "interrupted_detected"
	manualStopDisplayWindow       = 24 * time.Hour
	rescanWindow                  = 24 * time.Hour
	rescanMaxRolloutBytes         = 64 << 20
	// rescanMaxTotalBytes bounds the rollout bytes one rescan reads while the
	// watchdog holds its state lock; the directory walk itself is not bounded.
	rescanMaxTotalBytes = 512 << 20
)

func isManualStopReason(reason string) bool {
	return reason == stopReasonUserCancelled || reason == stopReasonInterruptedDetected
}

// attentionStoppedCount counts stopped tasks the tray should flag. Tasks the
// user cancelled, or that a rescan listed, are waiting for them on purpose.
func attentionStoppedCount(retries []ManagedRetry) int {
	count := 0
	for _, retry := range retries {
		if retry.State == "stopped" && !isManualStopReason(retry.StopReason) {
			count++
		}
	}
	return count
}

func stoppedDisplayWindow(stopped *StoppedRetry) time.Duration {
	if stopped != nil && isManualStopReason(stopped.Reason) {
		return manualStopDisplayWindow
	}
	return stoppedRetryDisplayWindow
}

func cancelledRetryStop(pending *PendingRetry, now time.Time) *StoppedRetry {
	return &StoppedRetry{
		EventKey: pending.EventKey, FailedTurnID: pending.FailedTurnID, FailedAt: pending.FailedAt,
		OriginTurnStartedAt: pending.OriginTurnStartedAt, Class: pending.Class, StoppedAt: now,
		CodexHome: pending.CodexHome, RolloutPath: pending.RolloutPath,
		Attempts: completedRetryCount(pending.Attempt), MaxAttempts: pending.MaxAttempts,
		ConsecutiveRetries: completedRetryCount(pending.ConsecutiveRetry), MaxConsecutive: pending.MaxConsecutive,
		Reason: stopReasonUserCancelled,
	}
}

type rescanCandidate struct {
	path      string
	root      sessionRoot
	modTime   time.Time
	size      int64
	preferred bool
}

// better prefers the Codex home the recovery transport serves (the restart
// path refuses any other home, so a mirror copy could never be resumed),
// then the most recently written file.
func (c rescanCandidate) better(other rescanCandidate) bool {
	if c.preferred != other.preferred {
		return c.preferred
	}
	return c.modTime.After(other.modTime)
}

// recoveryCodexHome mirrors how the shared recovery transport picks its home.
func recoveryCodexHome() string {
	if home := strings.TrimSpace(os.Getenv("CODEX_HOME")); home != "" {
		return filepath.Clean(expandPath(home))
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".codex")
	}
	return ""
}

// rescanInterruptedLocked lists recently active tasks whose last turn ended
// with a retryable failure and that nothing has continued since. It reads
// only lifecycle events, the same ones the regular scan reads.
func (d *daemon) rescanInterruptedLocked(now time.Time) int {
	latest := map[string]rescanCandidate{}
	recoveryHome := recoveryCodexHome()
	for _, root := range discoverSessionRoots(d.config) {
		preferred := recoveryHome != "" && strings.EqualFold(filepath.Clean(root.CodexHome), recoveryHome)
		// Archived tasks were put away by the user; only live sessions count.
		if strings.TrimSpace(root.Sessions) == "" {
			continue
		}
		for _, directory := range []string{root.Sessions} {
			_ = filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil || entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
					return nil
				}
				threadID := threadIDFromPath(path)
				if threadID == "" {
					return nil
				}
				info, err := entry.Info()
				if err != nil || info.Size() > rescanMaxRolloutBytes || now.Sub(info.ModTime()) > rescanWindow {
					return nil
				}
				candidate := rescanCandidate{path: path, root: root, modTime: info.ModTime(), size: info.Size(), preferred: preferred}
				if current, ok := latest[threadID]; !ok || candidate.better(current) {
					latest[threadID] = candidate
				}
				return nil
			})
		}
	}
	threadIDs := make([]string, 0, len(latest))
	for threadID := range latest {
		threadIDs = append(threadIDs, threadID)
	}
	// Newest first, so the read budget never drops the most recent tasks.
	sort.Slice(threadIDs, func(i, j int) bool {
		a, b := latest[threadIDs[i]], latest[threadIDs[j]]
		if !a.modTime.Equal(b.modTime) {
			return a.modTime.After(b.modTime)
		}
		return threadIDs[i] < threadIDs[j]
	})

	found := 0
	var readBytes int64
	for _, threadID := range threadIDs {
		candidate := latest[threadID]
		thread := d.state.Threads[threadID]
		if thread.Pending != nil || thread.Awaiting != nil || thread.GoalHeld || thread.GoalStatus == "active" ||
			(thread.Stopped != nil && stoppedRetryIsVisible(thread.Stopped, now)) {
			continue
		}
		if _, active := d.active[threadID]; active {
			continue
		}
		if readBytes+candidate.size > rescanMaxTotalBytes {
			d.logger.Printf("interrupted task rescan skipped thread=%s reason=byte_budget", shortThreadID(threadID))
			continue
		}
		readBytes += candidate.size
		events, _, err := readAppendedEvents(candidate.path, 0, threadID, candidate.root, false)
		if err != nil {
			continue
		}
		failure, startedAt, ok := lastTurnFailure(events, threadID)
		if !ok || failure.TurnID == thread.LastAbortedTurnID ||
			failure.Timestamp.IsZero() || now.Sub(failure.Timestamp) > rescanWindow {
			continue
		}
		decision := classifyCompletionFailure(failure, d.config)
		if !decision.Retry {
			continue
		}
		recoveryLimit, consecutiveLimit := retryLimitsForDecision(decision, d.config)
		key := eventKey(threadID, failure)
		d.state.ProcessedEvents[key] = now
		thread.Stopped = &StoppedRetry{
			EventKey: key, FailedTurnID: failure.TurnID, FailedAt: failure.Timestamp,
			OriginTurnStartedAt: startedAt, Class: decision.Class, StoppedAt: now,
			CodexHome: candidate.root.CodexHome, RolloutPath: filepath.Clean(candidate.path),
			MaxAttempts: recoveryLimit, MaxConsecutive: consecutiveLimit,
			Reason: stopReasonInterruptedDetected,
		}
		d.state.Threads[threadID] = thread
		found++
		d.logger.Printf("interrupted task listed thread=%s category=%s", shortThreadID(threadID), decision.Class)
	}
	d.logger.Printf("interrupted task rescan completed listed=%d candidates=%d", found, len(threadIDs))
	return found
}

// lastTurnFailure returns the final turn outcome when it is a failed
// completion with nothing after it: no new turn, user input, or abort.
func lastTurnFailure(events []scannedEvent, threadID string) (RelevantEvent, time.Time, bool) {
	var last RelevantEvent
	haveLast := false
	startedAt := map[string]time.Time{}
	for _, item := range events {
		if item.ThreadID != threadID {
			continue
		}
		switch item.Event.Kind {
		case "task_started":
			startedAt[item.Event.TurnID] = item.Event.Timestamp
			last, haveLast = item.Event, true
		case "task_complete", "turn_aborted", "task_user_input":
			last, haveLast = item.Event, true
		}
	}
	if !haveLast || last.Kind != "task_complete" || completionSucceeded(last) {
		return RelevantEvent{}, time.Time{}, false
	}
	return last, startedAt[last.TurnID], true
}
