package toggl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestClient returns a Client pointed at a server that answers every request
// with the given handler.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &Client{
		BaseURL:    srv.URL,
		apiToken:   "test-token",
		HttpClient: srv.Client(),
		Retry:      fastRetry(),
	}
}

// fastRetry keeps the real retry behaviour but shrinks the waits, so tests
// exercise the loop without sleeping through it.
func fastRetry() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   time.Millisecond,
		MaxDelay:    2 * time.Millisecond,
		Budget:      5 * time.Second,
	}
}

func respondWith(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}
}

// SendRequest must never hand back (nil, nil): callers dereference the returned
// message, so a nil with no error is a guaranteed panic one frame up.
func TestSendRequestNeverReturnsNilWithoutError(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"empty body", http.StatusOK, ""},
		{"whitespace body", http.StatusOK, "   "},
		{"html error page", http.StatusOK, "<html>nope</html>"},
		{"truncated json", http.StatusOK, `{"id":`},
		{"error status", http.StatusNotFound, "See https://engineering.toggl.com/docs/"},
		{"server error", http.StatusInternalServerError, "boom"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(t, respondWith(tc.status, tc.body))

			message, err := client.NewMessage("GET", "me/time_entries/current", nil)
			if err != nil {
				t.Fatal(err)
			}

			data, err := client.SendRequest(message)
			if data == nil && err == nil {
				t.Fatal("got (nil, nil); callers dereference the result and would panic")
			}
			if err == nil {
				t.Fatalf("expected an error for a %s, got none", tc.name)
			}
		})
	}
}

// The body of a failed request belongs in the error, not on stdout.
func TestSendRequestErrorCarriesResponseBody(t *testing.T) {
	client := newTestClient(t, respondWith(http.StatusNotFound, "no such workspace"))

	message, err := client.NewMessage("GET", "workspaces/1/projects", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.SendRequest(message)
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); got != "no such workspace, status code: 404" {
		t.Errorf("error = %q, want the response body and status", got)
	}
}

func TestSendRequestRateLimit(t *testing.T) {
	client := newTestClient(t, respondWith(http.StatusTooManyRequests, ""))

	message, err := client.NewMessage("GET", "me/time_entries", nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.SendRequest(message); err == nil {
		t.Fatal("expected a rate limit error")
	}
}

// v9 is inconsistent about collection responses: some endpoints return a bare
// array, the paginated ones wrap rows under "data" or "items". Confirmed live:
// /workspaces/{id}/projects is bare, /workspaces/{id}/tasks is wrapped.
func TestDecodeListAcceptsEveryCollectionShape(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		count int
	}{
		{"bare array", `[{"id":1},{"id":2}]`, 2},
		{"data envelope", `{"data":[{"id":1},{"id":2}],"total_count":2}`, 2},
		{"items envelope", `{"items":[{"id":1},{"id":2}],"total_count":2}`, 2},
		{"empty array", `[]`, 0},
		{"null data", `{"data":null,"total_count":0}`, 0},
		{"null body", `null`, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rows []Project
			if err := decodeList(json.RawMessage(tc.body), &rows); err != nil {
				t.Fatalf("decodeList(%s): %v", tc.body, err)
			}
			if len(rows) != tc.count {
				t.Errorf("decoded %d rows, want %d", len(rows), tc.count)
			}
		})
	}
}

func TestDecodeListRejectsUnexpectedShape(t *testing.T) {
	var rows []Project
	if err := decodeList(json.RawMessage(`"a string"`), &rows); err == nil {
		t.Fatal("expected an error for a non-collection response")
	}
}
