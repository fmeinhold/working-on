package toggl

import (
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// RetryPolicy controls how a failed request is repeated.
type RetryPolicy struct {
	// MaxAttempts is the total number of sends, not the number of retries.
	MaxAttempts int
	// BaseDelay is the first backoff wait; it doubles with each attempt.
	BaseDelay time.Duration
	// MaxDelay caps any single backoff wait.
	MaxDelay time.Duration
	// Budget bounds a whole request including its retries, so a hung API
	// cannot stall a command a shell prompt is waiting on.
	Budget time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   250 * time.Millisecond,
		MaxDelay:    2 * time.Second,
		Budget:      45 * time.Second,
	}
}

// withDefaults fills in anything left zero, so a directly constructed Client
// still retries sensibly.
func (p RetryPolicy) withDefaults() RetryPolicy {
	defaults := DefaultRetryPolicy()

	if p.MaxAttempts <= 0 {
		p.MaxAttempts = defaults.MaxAttempts
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = defaults.BaseDelay
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = defaults.MaxDelay
	}
	if p.Budget <= 0 {
		p.Budget = defaults.Budget
	}

	return p
}

// delay is the wait before the given attempt is repeated. It is exponential
// with jitter, so concurrent clients do not retry in lockstep. A Retry-After
// from the server wins outright - it knows better than we do.
func (p RetryPolicy) delay(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return retryAfter
	}

	backoff := p.BaseDelay << (attempt - 1)
	if backoff > p.MaxDelay || backoff <= 0 {
		backoff = p.MaxDelay
	}

	// Jitter across the upper half, keeping a sensible floor.
	half := backoff / 2
	return half + time.Duration(rand.Int63n(int64(half)+1))
}

// attemptOutcome describes why an attempt failed and whether repeating it is
// worthwhile.
type attemptOutcome struct {
	retryable  bool
	retryAfter time.Duration
}

// isIdempotent reports whether repeating a request is safe.
//
// POST creates a time entry. If a POST fails ambiguously - a dropped
// connection, a 502 - the server may well have created the entry anyway, and
// repeating it would book the same hours twice. For a tool that bills time
// that is far worse than surfacing the error and letting the user re-run.
func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// parseRetryAfter reads a Retry-After header, which is either a number of
// seconds or an HTTP date.
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}

	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}

	if when, err := http.ParseTime(value); err == nil {
		if wait := time.Until(when); wait > 0 {
			return wait
		}
	}

	return 0
}
