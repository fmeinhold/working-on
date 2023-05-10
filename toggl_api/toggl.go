package toggl_api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	baseURL = "https://api.track.toggl.com/api/v9/"
	timeout = 30 * time.Second
)

type Toggl struct {
	Me            *MeClient
	Projects      *ProjectsClient
	Tasks         *TasksClient
	Workspaces    *WorkspacesClient
	TimeEntries   *TimeEntriesClient
	requestConfig *RequestConfig
}

type RequestConfig struct {
	apiToken string
	baseURL  string
	client   *http.Client
}

func NewToggl(apiToken string) *Toggl {
	requestConfig := &RequestConfig{
		apiToken: apiToken,
		client: &http.Client{
			Timeout: time.Minute,
		},
	}
	return &Toggl{
		Me:            NewMeClient(requestConfig),
		Workspaces:    &WorkspacesClient{config: requestConfig},
		Projects:      &ProjectsClient{requestConfig},
		Tasks:         &TasksClient{requestConfig},
		TimeEntries:   &TimeEntriesClient{requestConfig},
		requestConfig: requestConfig,
	}
}

func (r *RequestConfig) SendRequest(method string, endpoint string, body io.Reader) (*json.RawMessage, error) {
	var (
		req *http.Request
		err error
	)

	req, err = http.NewRequest(method, baseURL+endpoint, body)

	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req = req.WithContext(ctx)
	req.SetBasicAuth(r.apiToken, "api_token")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json; charset=utf-8")

	res, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(res.Body)

	if res.StatusCode == http.StatusTooManyRequests {
		return nil, errors.New("rate limit hit")
	}

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusBadRequest {
		b, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%s, status code: %d, %s", b, res.StatusCode, baseURL+endpoint)
	}

	js, err := io.ReadAll(res.Body)

	var raw json.RawMessage
	if json.Unmarshal(js, &raw) != nil {
		return nil, err
	}

	return &raw, err

}
