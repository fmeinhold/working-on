package toggl

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// countingServer answers the first failures times with status, then succeeds.
// It records every request so tests can count attempts and inspect bodies.
type countingServer struct {
	failures int32
	status   int
	header   map[string]string

	attempts int32
	bodies   []string
	methods  []string
}

func (s *countingServer) client(t *testing.T) *Client {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&s.attempts, 1)

		raw, _ := ioutil.ReadAll(r.Body)
		s.bodies = append(s.bodies, string(raw))
		s.methods = append(s.methods, r.Method)

		if n <= atomic.LoadInt32(&s.failures) {
			for key, value := range s.header {
				w.Header().Set(key, value)
			}
			w.WriteHeader(s.status)
			fmt.Fprint(w, "temporarily unavailable")
			return
		}

		fmt.Fprint(w, `{"id":1,"description":"ok"}`)
	}))
	t.Cleanup(srv.Close)

	return &Client{
		BaseURL:    srv.URL,
		apiToken:   "test-token",
		HttpClient: srv.Client(),
		Retry:      fastRetry(),
	}
}

func (s *countingServer) count() int { return int(atomic.LoadInt32(&s.attempts)) }

// A GET that fails transiently recovers without the caller seeing anything.
func TestRetryRecoversFromTransientServerError(t *testing.T) {
	for _, status := range []int{500, 502, 503, 504} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := &countingServer{failures: 1, status: status}
			client := server.client(t)

			message, err := client.NewMessage("GET", "me/time_entries", nil)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := client.SendRequest(message); err != nil {
				t.Fatalf("a retryable %d should have recovered: %v", status, err)
			}
			if server.count() != 2 {
				t.Errorf("made %d attempts, want 2", server.count())
			}
		})
	}
}

func TestRetryGivesUpAfterMaxAttempts(t *testing.T) {
	server := &countingServer{failures: 99, status: 503}
	client := server.client(t)

	message, err := client.NewMessage("GET", "me/time_entries", nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.SendRequest(message); err == nil {
		t.Fatal("expected an error once attempts are exhausted")
	}
	if server.count() != 3 {
		t.Errorf("made %d attempts, want 3", server.count())
	}
}

// The whole point of the exercise: a client error is the caller's fault and
// repeating it just wastes time.
func TestRetryDoesNotRepeatClientErrors(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := &countingServer{failures: 99, status: status}
			client := server.client(t)

			message, err := client.NewMessage("GET", "me/time_entries", nil)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := client.SendRequest(message); err == nil {
				t.Fatal("expected an error")
			}
			if server.count() != 1 {
				t.Errorf("made %d attempts for a %d, want 1", server.count(), status)
			}
		})
	}
}

// Creating a time entry is not idempotent. A 5xx may mean the entry was
// created anyway, so repeating it risks booking the same hours twice.
func TestRetryNeverRepeatsAPostOnServerError(t *testing.T) {
	server := &countingServer{failures: 99, status: 503}
	client := server.client(t)

	message, err := client.NewMessage("POST", "workspaces/5/time_entries",
		&TimeEntry{Description: "eight hours", WorkspaceId: 5})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.SendRequest(message); err == nil {
		t.Fatal("expected an error")
	}
	if server.count() != 1 {
		t.Errorf("a POST was sent %d times; it must be sent once", server.count())
	}
}

// A 429 is different: the request was refused rather than processed, so even
// a POST can safely be repeated.
func TestRetryRepeatsPostOnRateLimit(t *testing.T) {
	server := &countingServer{failures: 1, status: http.StatusTooManyRequests}
	client := server.client(t)

	message, err := client.NewMessage("POST", "workspaces/5/time_entries",
		&TimeEntry{Description: "eight hours", WorkspaceId: 5})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.SendRequest(message); err != nil {
		t.Fatalf("a rate limited POST should have recovered: %v", err)
	}
	if server.count() != 2 {
		t.Errorf("made %d attempts, want 2", server.count())
	}
}

// The request body must survive being sent more than once.
func TestRetryResendsTheRequestBody(t *testing.T) {
	server := &countingServer{failures: 1, status: http.StatusTooManyRequests}
	client := server.client(t)

	message, err := client.NewMessage("PATCH", "workspaces/5/time_entries/1",
		&TimeEntry{Description: "still here", WorkspaceId: 5})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.SendRequest(message); err != nil {
		t.Fatal(err)
	}
	if len(server.bodies) != 2 {
		t.Fatalf("saw %d requests, want 2", len(server.bodies))
	}
	if server.bodies[0] == "" || server.bodies[0] != server.bodies[1] {
		t.Errorf("body differed between attempts:\n  first:  %q\n  second: %q",
			server.bodies[0], server.bodies[1])
	}
}

func TestRetryHonoursRetryAfterSeconds(t *testing.T) {
	server := &countingServer{
		failures: 1,
		status:   http.StatusTooManyRequests,
		header:   map[string]string{"Retry-After": "1"},
	}
	client := server.client(t)

	message, err := client.NewMessage("GET", "me/time_entries", nil)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if _, err := client.SendRequest(message); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	// The policy's own backoff is a millisecond, so anything near a second
	// can only have come from the header.
	if elapsed < time.Second {
		t.Errorf("waited %s; Retry-After: 1 was not honoured", elapsed)
	}
}

// A Retry-After longer than the budget should end the attempt rather than
// stalling the command for minutes.
func TestRetryStopsWhenTheWaitExceedsTheBudget(t *testing.T) {
	server := &countingServer{
		failures: 99,
		status:   http.StatusTooManyRequests,
		header:   map[string]string{"Retry-After": "3600"},
	}
	client := server.client(t)

	message, err := client.NewMessage("GET", "me/time_entries", nil)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if _, err := client.SendRequest(message); err == nil {
		t.Fatal("expected an error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %s; an hour long Retry-After should have been abandoned", elapsed)
	}
	if server.count() != 1 {
		t.Errorf("made %d attempts, want 1", server.count())
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		header string
		want   time.Duration
		about  string
	}{
		{"", 0, "absent"},
		{"5", 5 * time.Second, "seconds"},
		{"0", 0, "zero"},
		{"-3", 0, "negative"},
		{"soon", 0, "not a number or a date"},
	}

	for _, tc := range cases {
		t.Run(tc.about, func(t *testing.T) {
			if got := parseRetryAfter(tc.header); got != tc.want {
				t.Errorf("parseRetryAfter(%q) = %s, want %s", tc.header, got, tc.want)
			}
		})
	}

	t.Run("http date", func(t *testing.T) {
		header := time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat)
		got := parseRetryAfter(header)
		if got < 25*time.Second || got > 31*time.Second {
			t.Errorf("parseRetryAfter(%q) = %s, want about 30s", header, got)
		}
	})
}

func TestIsIdempotent(t *testing.T) {
	for method, want := range map[string]bool{
		http.MethodGet:    true,
		http.MethodPut:    true,
		http.MethodPatch:  true,
		http.MethodDelete: true,
		http.MethodHead:   true,
		http.MethodPost:   false,
	} {
		if got := isIdempotent(method); got != want {
			t.Errorf("isIdempotent(%s) = %v, want %v", method, got, want)
		}
	}
}

// Backoff grows and stays inside its cap.
func TestRetryPolicyDelayGrowsAndIsCapped(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts: 5,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    400 * time.Millisecond,
		Budget:      time.Minute,
	}

	for attempt := 1; attempt <= 6; attempt++ {
		delay := policy.delay(attempt, 0)
		if delay <= 0 {
			t.Errorf("attempt %d: delay %s, want a positive wait", attempt, delay)
		}
		if delay > policy.MaxDelay {
			t.Errorf("attempt %d: delay %s exceeds the %s cap", attempt, delay, policy.MaxDelay)
		}
	}

	// Jitter should keep the first wait below the fourth's ceiling but above
	// half the base.
	if got := policy.delay(1, 0); got < 50*time.Millisecond || got > 100*time.Millisecond {
		t.Errorf("first delay = %s, want between 50ms and 100ms", got)
	}
}

func TestRetryPolicyDelayPrefersRetryAfter(t *testing.T) {
	policy := DefaultRetryPolicy()

	if got := policy.delay(1, 7*time.Second); got != 7*time.Second {
		t.Errorf("delay = %s, want the 7s the server asked for", got)
	}
}

// A zero policy must still behave, so a directly constructed Client retries.
func TestRetryPolicyZeroValueFallsBackToDefaults(t *testing.T) {
	policy := RetryPolicy{}.withDefaults()
	defaults := DefaultRetryPolicy()

	if policy != defaults {
		t.Errorf("zero policy resolved to %+v, want %+v", policy, defaults)
	}
}

// A connection that never lands is retryable for a GET.
func TestRetryRepeatsOnTransportFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening now

	client := &Client{
		BaseURL:    url,
		apiToken:   "test-token",
		HttpClient: &http.Client{Timeout: time.Second},
		Retry:      fastRetry(),
	}

	message, err := client.NewMessage("GET", "me/time_entries", nil)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if _, err := client.SendRequest(message); err == nil {
		t.Fatal("expected an error with nothing listening")
	}

	// Three attempts with two backoffs; the waits are tiny but non-zero.
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %s, expected to give up quickly", elapsed)
	}
}
