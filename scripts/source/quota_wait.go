package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// desktopTransportCheckTimeout bounds the PowerShell process scan that proves
// which transport Codex Desktop uses. A busy machine routinely needs more than
// the previous 8 seconds, and every timeout was reported as the generic
// controller_unavailable.
const desktopTransportCheckTimeout = 20 * time.Second

// lifecycleProbeTimeout must exceed the transport check it contains.
// Shared mode checks the transport twice, so allow two checks plus slack. The
// same budget bounds the readiness probe that gates dispatch.
const lifecycleProbeTimeout = 2*desktopTransportCheckTimeout + 5*time.Second

const (
	// quotaResetGrace is added after the announced reset so the retry does not
	// race the provider's own clock.
	quotaResetGrace = 30 * time.Second
	// quotaResetHorizon ignores announced resets further away than the longest
	// plan window; a parse of anything beyond it is more likely a mistake.
	quotaResetHorizon = 8 * 24 * time.Hour
	// quotaProbeBackoffCap bounds the pause between failed lifecycle probes.
	quotaProbeBackoffCap = 5 * time.Minute
)

var usageLimitResetPattern = regexp.MustCompile(
	`(?i)try again at\s+(?:([a-z]{3,9})\s+(\d{1,2})(?:st|nd|rd|th)?,?\s+(?:(\d{4})\s+)?)?(\d{1,2}):(\d{2})\s*([ap]m)`)

// usageLimitResetAt reads the "try again at ..." sentence Codex appends to a
// usage-limit error. The clock is the machine's local time; a bare time that
// has already passed today means tomorrow.
func usageLimitResetAt(text string, now time.Time, loc *time.Location) (time.Time, bool) {
	match := usageLimitResetPattern.FindStringSubmatch(text)
	if match == nil {
		return time.Time{}, false
	}
	hour, _ := strconv.Atoi(match[4])
	minute, _ := strconv.Atoi(match[5])
	if hour < 1 || hour > 12 || minute > 59 {
		return time.Time{}, false
	}
	hour %= 12
	if strings.EqualFold(match[6], "pm") {
		hour += 12
	}
	local := now.In(loc)
	if match[1] == "" {
		reset := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, loc)
		if !reset.After(now) {
			reset = reset.AddDate(0, 0, 1)
		}
		return reset, true
	}
	month, err := time.Parse("Jan", match[1][:3])
	if err != nil {
		return time.Time{}, false
	}
	day, _ := strconv.Atoi(match[2])
	year := local.Year()
	explicitYear := match[3] != ""
	if explicitYear {
		year, _ = strconv.Atoi(match[3])
	}
	reset := time.Date(year, month.Month(), day, hour, minute, 0, 0, loc)
	if reset.Day() != day {
		return time.Time{}, false
	}
	if !explicitYear && reset.Before(now.Add(-24*time.Hour)) {
		reset = reset.AddDate(1, 0, 0)
	}
	return reset, true
}

// quotaResetFromError returns the announced reset when it is a usable future
// time within the plausible horizon. A bare clock time is read against the
// moment the error happened, not the moment the daemon processed it.
func quotaResetFromError(text string, eventAt, now time.Time) (time.Time, bool) {
	reference := eventAt
	if reference.IsZero() || reference.After(now) {
		reference = now
	}
	reset, ok := usageLimitResetAt(text, reference, time.Local)
	if !ok || !reset.After(now) || reset.Sub(now) > quotaResetHorizon {
		return time.Time{}, false
	}
	return reset, true
}

// quotaControllerWaitable reports whether a controller failure is the kind a
// quota-limited thread should simply wait out. Codex being closed or restarting
// is temporary; configuration conflicts are not and still stop the retry.
func quotaControllerWaitable(reason string) bool {
	return !controllerFailureNeedsAction(reason) || reason == "codex_not_running" || reason == "codex_restart_required"
}

// quotaNotSentReason lists dispatch failures that prove no turn reached Codex,
// so a quota-limited task can wait them out without risking a duplicate turn.
func quotaNotSentReason(reason string) bool {
	switch reason {
	case "codex_thread_owner_unavailable", "codex_not_running", "codex_restart_required", "codex_background_channel_unavailable":
		return true
	default:
		return false
	}
}

// quotaAmbiguousReason lists failures that may have happened after a turn was
// sent. They get a larger but finite failure limit.
func quotaAmbiguousReason(reason string) bool {
	return reason == "controller_timeout" || reason == "controller_unavailable"
}

const quotaAmbiguousLimitFactor = 4

// recoveryWindowStart is when the automatic recovery window begins. A thread
// waiting for its quota to reset must not burn the window while it waits.
func recoveryWindowStart(thread ThreadState) time.Time {
	if thread.QuotaResetAt.After(thread.RecoveryStartedAt) {
		return thread.QuotaResetAt
	}
	return thread.RecoveryStartedAt
}

func lifecycleProbeBackoff(failures int) time.Duration {
	if failures > 6 {
		failures = 6
	}
	delay := awaitingLifecycleProbeInterval << uint(failures)
	if delay > quotaProbeBackoffCap {
		delay = quotaProbeBackoffCap
	}
	return delay
}

// wrapControllerCommandError marks a command failure caused by its own
// deadline as a controller timeout instead of a generic outage.
func wrapControllerCommandError(ctx context.Context, err error) error {
	if err != nil && ctx.Err() != nil && !errors.Is(err, errControllerTimeout) {
		return fmt.Errorf("%w: %v", errControllerTimeout, err)
	}
	return err
}
