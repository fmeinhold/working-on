package toggl

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"
	"time"
)

// request records what the client actually put on the wire.
type request struct {
	method string
	path   string
	query  string
	body   map[string]interface{}
}

// recordRequest captures each request and replies with the body produced by
// reply, which receives the 1-based request number so multi-call flows can
// answer differently per step.
func recordRequest(t *testing.T, reply func(n int) string) (*TimeEntries, *TaskClient, *WorkspaceClient, *request) {
	t.Helper()

	last := &request{}
	n := 0

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n++
		last.method, last.path, last.query = r.Method, r.URL.Path, r.URL.RawQuery
		last.body = nil

		if raw, _ := ioutil.ReadAll(r.Body); len(raw) > 0 {
			_ = json.Unmarshal(raw, &last.body)
		}

		if _, err := w.Write([]byte(reply(n))); err != nil {
			t.Error(err)
		}
	})

	return &TimeEntries{client: client}, &TaskClient{client: client}, &WorkspaceClient{client: client}, last
}

func TestAddPostsUnwrappedToWorkspaceEndpoint(t *testing.T) {
	entries, _, _, req := recordRequest(t, func(int) string {
		return `{"id":99,"workspace_id":123,"project_id":456,"description":"hacking","duration":3600}`
	})

	start := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	stop := start.Add(time.Hour)

	got, err := entries.Add(&TimeEntry{
		Description: "hacking",
		WorkspaceId: 123,
		ProjectId:   456,
		TaskId:      7,
		Start:       &start,
		Stop:        &stop,
		Duration:    3600,
		CreatedWith: CreatedWith,
	})
	if err != nil {
		t.Fatal(err)
	}

	if req.method != "POST" {
		t.Errorf("method = %s, want POST", req.method)
	}
	if req.path != "/workspaces/123/time_entries" {
		t.Errorf("path = %s, want /workspaces/123/time_entries", req.path)
	}

	for field, want := range map[string]interface{}{
		"workspace_id": 123.0,
		"project_id":   456.0,
		"task_id":      7.0,
		"duration":     3600.0,
		"created_with": "working_on",
		"start":        "2026-08-06T09:00:00Z",
	} {
		if req.body[field] != want {
			t.Errorf("body[%q] = %v, want %v", field, req.body[field], want)
		}
	}

	// v8 wrapped the payload and used short id names; v9 does neither.
	for _, gone := range []string{"time_entry", "wid", "pid", "tid"} {
		if _, present := req.body[gone]; present {
			t.Errorf("v8 key %q is still being sent", gone)
		}
	}

	// The response is the entry itself, not {"data": ...}.
	if got.Id != 99 || got.ProjectId != 456 {
		t.Errorf("decoded %+v, want id 99 and project 456", got)
	}
}

func TestAddRequiresWorkspace(t *testing.T) {
	entries, _, _, _ := recordRequest(t, func(int) string { return `{}` })

	start := time.Now()
	if _, err := entries.Add(&TimeEntry{Description: "x", Start: &start}); err == nil {
		t.Fatal("expected an error when workspace id is unset")
	}
}

// A running entry is duration -1 with no stop. v8 used the negative unix start
// time, which v9 would read as a nonsensically long entry.
func TestStartMarksEntryRunning(t *testing.T) {
	entries, _, _, req := recordRequest(t, func(int) string {
		return `{"id":1,"workspace_id":5,"duration":-1,"stop":null}`
	})

	start := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	got, err := entries.Start(&TimeEntry{
		Description: "live",
		WorkspaceId: 5,
		Start:       &start,
		Duration:    RunningDuration,
		CreatedWith: CreatedWith,
	})
	if err != nil {
		t.Fatal(err)
	}

	if req.body["duration"] != -1.0 {
		t.Errorf("duration = %v, want -1", req.body["duration"])
	}
	if _, present := req.body["stop"]; present {
		t.Error("stop must be omitted while the timer runs")
	}
	if !got.IsRunning() {
		t.Error("IsRunning() = false for a duration of -1")
	}
}

func TestCurrentReturnsNilWhenNothingRunning(t *testing.T) {
	for _, body := range []string{`null`, `{}`} {
		entries, _, _, req := recordRequest(t, func(int) string { return body })

		got, err := entries.Current()
		if err != nil {
			t.Fatalf("body %s: %v", body, err)
		}
		if got != nil {
			t.Errorf("body %s: got %+v, want nil", body, got)
		}
		if req.path != "/me/time_entries/current" {
			t.Errorf("path = %s, want /me/time_entries/current", req.path)
		}
	}
}

func TestCurrentDecodesRunningEntry(t *testing.T) {
	entries, _, _, _ := recordRequest(t, func(int) string {
		return `{"id":7,"workspace_id":5,"project_id":91,"description":"live","duration":-1}`
	})

	got, err := entries.Current()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Id != 7 || got.ProjectId != 91 || !got.IsRunning() {
		t.Fatalf("got %+v, want the running entry 7 on project 91", got)
	}
}

// Stopping is workspace-scoped in v9. The workspace comes from the entry
// itself, so stopping does not depend on config being correct.
func TestStopCurrentUsesWorkspaceFromEntry(t *testing.T) {
	entries, _, _, req := recordRequest(t, func(n int) string {
		if n == 1 {
			return `{"id":777,"workspace_id":42,"duration":-1}`
		}
		return `{"id":777,"workspace_id":42,"duration":1800,"stop":"2026-08-06T10:00:00Z"}`
	})

	got, err := entries.StopCurrent()
	if err != nil {
		t.Fatal(err)
	}

	if req.method != "PATCH" {
		t.Errorf("method = %s, want PATCH", req.method)
	}
	if req.path != "/workspaces/42/time_entries/777/stop" {
		t.Errorf("path = %s, want /workspaces/42/time_entries/777/stop", req.path)
	}
	if got.Duration != 1800 {
		t.Errorf("duration = %d, want 1800", got.Duration)
	}
}

func TestStopCurrentWithoutRunningEntry(t *testing.T) {
	entries, _, _, _ := recordRequest(t, func(int) string { return `null` })

	if _, err := entries.StopCurrent(); err == nil {
		t.Fatal("expected an error when no timer is running")
	}
}

func TestListSendsDateRange(t *testing.T) {
	entries, _, _, req := recordRequest(t, func(int) string {
		return `[{"id":1,"description":"a"},{"id":2,"description":"b"}]`
	})

	start := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	list, err := entries.List(&start, &end)
	if err != nil {
		t.Fatal(err)
	}

	if req.path != "/me/time_entries" {
		t.Errorf("path = %s, want /me/time_entries", req.path)
	}
	want := "end_date=2026-08-07T00%3A00%3A00Z&start_date=2026-08-06T00%3A00%3A00Z"
	if req.query != want {
		t.Errorf("query = %s, want %s", req.query, want)
	}
	if list.Count != 2 {
		t.Errorf("count = %d, want 2", list.Count)
	}
}

func TestListWithoutDatesOmitsQuery(t *testing.T) {
	entries, _, _, req := recordRequest(t, func(int) string { return `[]` })

	if _, err := entries.List(nil, nil); err != nil {
		t.Fatal(err)
	}
	if req.query != "" {
		t.Errorf("query = %q, want empty", req.query)
	}
}

// v9 does not document the sort order of the listing, so MostRecent must not
// depend on it.
func TestMostRecentIsSortOrderIndependent(t *testing.T) {
	cases := map[string]string{
		"oldest first": `[{"id":1,"start":"2026-08-01T09:00:00Z"},{"id":2,"start":"2026-08-06T09:00:00Z"}]`,
		"newest first": `[{"id":2,"start":"2026-08-06T09:00:00Z"},{"id":1,"start":"2026-08-01T09:00:00Z"}]`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			entries, _, _, _ := recordRequest(t, func(int) string { return body })

			got, err := entries.MostRecent()
			if err != nil {
				t.Fatal(err)
			}
			if got == nil || got.Id != 2 {
				t.Fatalf("got %+v, want entry 2", got)
			}
		})
	}
}

func TestMostRecentOnEmptyList(t *testing.T) {
	entries, _, _, _ := recordRequest(t, func(int) string { return `[]` })

	got, err := entries.MostRecent()
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("got %+v, want nil for an empty list", got)
	}
}

func TestValidateComputesDurationFromStopTime(t *testing.T) {
	start := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	stop := start.Add(90 * time.Minute)

	entry := &TimeEntry{WorkspaceId: 5, Start: &start, Stop: &stop}
	if err := entry.Validate(); err != nil {
		t.Fatal(err)
	}
	if entry.Duration != 5400 {
		t.Errorf("duration = %d, want 5400", entry.Duration)
	}
}

func TestValidateRejectsIncompleteEntries(t *testing.T) {
	start := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	stop := start.Add(time.Hour)

	cases := map[string]*TimeEntry{
		"no workspace":            {Start: &start, Stop: &stop},
		"no start":                {WorkspaceId: 5, Stop: &stop},
		"no stop and no duration": {WorkspaceId: 5, Start: &start},
	}

	for name, entry := range cases {
		t.Run(name, func(t *testing.T) {
			if err := entry.Validate(); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestUpdatePutsTheWholeEntry(t *testing.T) {
	entries, _, _, req := recordRequest(t, func(int) string {
		return `{"id":42,"workspace_id":7,"description":"named at last","duration":-1}`
	})

	start := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)

	got, err := entries.Update(&TimeEntry{
		Id:          42,
		WorkspaceId: 7,
		Description: "named at last",
		Start:       &start,
		Duration:    RunningDuration,
	})
	if err != nil {
		t.Fatal(err)
	}

	if req.method != "PUT" {
		t.Errorf("method = %s, want PUT", req.method)
	}
	if req.path != "/workspaces/7/time_entries/42" {
		t.Errorf("path = %s, want /workspaces/7/time_entries/42", req.path)
	}
	if req.body["description"] != "named at last" {
		t.Errorf("body[description] = %v, want %q", req.body["description"], "named at last")
	}

	// The entry was running, and saving a description must not end it.
	if req.body["duration"] != float64(RunningDuration) {
		t.Errorf("body[duration] = %v, want %d", req.body["duration"], RunningDuration)
	}
	if _, stopped := req.body["stop"]; stopped {
		t.Error("a stop was sent for a running entry")
	}

	if got.Description != "named at last" {
		t.Errorf("decoded %+v, want the updated description", got)
	}
}

func TestUpdateNeedsAnIdentifiedEntry(t *testing.T) {
	entries, _, _, _ := recordRequest(t, func(int) string { return `{}` })

	if _, err := entries.Update(&TimeEntry{WorkspaceId: 7}); err == nil {
		t.Error("expected an error when the entry has no id")
	}
	if _, err := entries.Update(&TimeEntry{Id: 42}); err == nil {
		t.Error("expected an error when the entry has no workspace")
	}
}
