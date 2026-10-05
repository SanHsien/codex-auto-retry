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
)

func isManualStopReason(reason string) bool {
	return reason == stopReasonUserCancelled || reason == stopReasonInterruptedDetected
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
	path    string
	root    sessionRoot
	modTime time.Time
	size    int64
}

// rescanInterruptedLocked lists recently active tasks whose last turn ended
// with a retryable failure and that nothing has continued since. It reads
// only lifecycle events, the same ones the regular scan reads.
func (d *daemon) rescanInterruptedLocked(now time.Time) int {
	latest := map[string]rescanCandidate{}
	for _, root := range discoverSessionRoots(d.config) {
		for _, directory := range root.scanDirectories() {
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
				if current, ok := latest[threadID]; !ok || info.ModTime().After(current.modTime) {
					latest[threadID] = rescanCandidate{path: path, root: root, modTime: info.ModTime(), size: info.Size()}
				}
				return nil
			})
		}
	}
	threadIDs := make([]string, 0, len(latest))
	for threadID := range latest {
		threadIDs = append(threadIDs, threadID)
	}
	sort.Strings(threadIDs)

	found := 0
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
		events, _, err := readAppendedEvents(candidate.path, 0, threadID, candidate.root, false)
		if err != nil {
			continue
		}
		failure, startedAt, ok := lastTurnFailure(events, threadID)
		if !ok || failure.TurnID == thread.LastAbortedTurnID {
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
