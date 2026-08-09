package toggl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

const (
	BaseURL        = "https://api.track.toggl.com/api/v9"
	requestTimeout = 30 * time.Second
)

type Toggl struct {
	TimeEntries     *TimeEntries
	TaskClient      *TaskClient
	WorkspaceClient *WorkspaceClient
}

type Client struct {
	BaseURL    string
	apiToken   string
	HttpClient *http.Client

	// Retry controls how failed requests are repeated. A zero value falls
	// back to DefaultRetryPolicy.
	Retry RetryPolicy
}

type Message struct {
	endpoint string
	method   string
	payload  *bytes.Buffer
}

func NewToggl(apiToken string) *Toggl {
	return NewTogglAt(apiToken, BaseURL)
}

// NewTogglAt is NewToggl against a different base url, for tests and for
// pointing the client at a proxy.
func NewTogglAt(apiToken string, baseURL string) *Toggl {
	client := Client{
		BaseURL:  baseURL,
		apiToken: apiToken,
		HttpClient: &http.Client{
			Timeout: time.Minute,
		},
		Retry: DefaultRetryPolicy(),
	}
	return &Toggl{
		TimeEntries: &TimeEntries{
			client: &client,
		},
		TaskClient: &TaskClient{
			client: &client,
		},
		WorkspaceClient: &WorkspaceClient{
			client: &client,
		},
	}
}

func (c *Client) NewMessage(method string, endpoint string, data interface{}) (*Message, error) {
	payload := new(bytes.Buffer)
	if data != nil {
		err := json.NewEncoder(payload).Encode(data)
		if err != nil {
			return nil, err
		}
	}

	return &Message{
		endpoint: fmt.Sprintf("%s/%s", c.BaseURL, endpoint),
		method:   method,
		payload:  payload,
	}, nil

}

// Only requests that are safe to repeat are retried; see isIdempotent. The
// whole call, retries included, is bounded by the retry budget.
func (c *Client) SendRequest(message *Message) (*json.RawMessage, error) {
	policy := c.Retry.withDefaults()
	deadline := time.Now().Add(policy.Budget)

	// Snapshot the body once: the buffer is drained by the first send, so a
	// retry would otherwise post nothing at all.
	var payload []byte
	if message.payload != nil {
		payload = message.payload.Bytes()
	}

	var lastErr error

	for attempt := 1; ; attempt++ {
		raw, outcome, err := c.attempt(message, payload)
		if err == nil {
			return raw, nil
		}
		lastErr = err

		if !outcome.retryable || attempt >= policy.MaxAttempts {
			break
		}

		wait := policy.delay(attempt, outcome.retryAfter)
		if time.Now().Add(wait).After(deadline) {
			break
		}

		time.Sleep(wait)
	}

	return nil, lastErr
}

func (c *Client) attempt(message *Message, payload []byte) (*json.RawMessage, attemptOutcome, error) {
	var body io.Reader
	if len(payload) > 0 {
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(message.method, message.endpoint, body)
	if err != nil {
		return nil, attemptOutcome{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	req = req.WithContext(ctx)
	req.SetBasicAuth(c.apiToken, "api_token")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json; charset=utf-8")

	res, err := c.HttpClient.Do(req)
	if err != nil {
		// No response reached us. For an idempotent request that is worth
		// repeating; for a POST we cannot tell whether it took effect.
		return nil, attemptOutcome{retryable: isIdempotent(message.method)}, err
	}

	defer res.Body.Close()

	responseBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, attemptOutcome{retryable: isIdempotent(message.method)},
			fmt.Errorf("unable to read response body: %w", err)
	}

	// A rate limited request was refused rather than processed, so repeating
	// it is safe whatever the method.
	if res.StatusCode == http.StatusTooManyRequests {
		return nil, attemptOutcome{
			retryable:  true,
			retryAfter: parseRetryAfter(res.Header.Get("Retry-After")),
		}, errors.New("rate limit hit")
	}

	if res.StatusCode >= http.StatusInternalServerError {
		return nil, attemptOutcome{retryable: isIdempotent(message.method)},
			fmt.Errorf("%s, status code: %d", responseBody, res.StatusCode)
	}

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusBadRequest {
		return nil, attemptOutcome{}, fmt.Errorf("%s, status code: %d", responseBody, res.StatusCode)
	}

	var raw json.RawMessage
	if err := json.Unmarshal(responseBody, &raw); err != nil {
		return nil, attemptOutcome{}, fmt.Errorf("invalid json in response (status %d): %w", res.StatusCode, err)
	}

	return &raw, attemptOutcome{}, nil
}

// decodeList unmarshals a v9 collection response into out, which must be a
// pointer to a slice.
//
// v9 is not consistent about this: the plain collection endpoints return a bare
// JSON array, while the paginated ones wrap the rows in an object under "data"
// or "items". Accepting all three keeps us working whichever shape an endpoint
// happens to use, and survives an endpoint gaining pagination later.
func decodeList(raw json.RawMessage, out interface{}) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}

	if trimmed[0] == '[' {
		return json.Unmarshal(trimmed, out)
	}

	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		return fmt.Errorf("unexpected collection response shape: %w", err)
	}

	rows := envelope.Data
	if len(rows) == 0 {
		rows = envelope.Items
	}
	if len(rows) == 0 {
		return nil
	}

	return json.Unmarshal(rows, out)
}
