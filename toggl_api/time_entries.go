package toggl_api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

const (
	CreatedWith = "working-on"
)

type TimeEntriesClient struct {
	config *RequestConfig
}

type TimeEntry struct {
	Id          int        `json:"id,omitempty"`
	Description string     `json:"description"`
	WorkspaceID int        `json:"workspace_id"`
	ProjectID   int        `json:"project_id,omitempty"`
	TaskID      int        `json:"task_id,omitempty"`
	Billable    bool       `json:"billable"`
	Start       *time.Time `json:"start"`
	Stop        *time.Time `json:"stop,omitempty"`
	Duration    int64      `json:"duration,omitempty"`
	CreatedWith string     `json:"created_with"`
	Tags        []string   `json:"tags,omitempty"`
	At          *time.Time `json:"at,omitempty"`
}

func (t TimeEntry) String() string {
	//isRunning := t.Duration < 0

	return fmt.Sprintf("'%s' %s, %s (tid: %d)", t.Description, t.Start, time.Duration(t.Duration)*time.Second, t.Id)
}

func (t TimeEntry) IsSet() bool {
	return t.Id > 0
}

func NewTimeEntryRunning(wid int, pid int, description string, running bool, billable bool, start *time.Time) *TimeEntry {
	loc, _ := time.LoadLocation("UTC")

	if start == nil {
		now := time.Now().In(loc)
		start = &now
	}

	var duration = start.Unix()

	if running {
		duration = -1 * duration
	}

	timeEntry := &TimeEntry{
		Id:          0,
		Description: description,
		WorkspaceID: wid,
		ProjectID:   pid,
		Billable:    billable,
		Start:       start,
		Stop:        nil,
		Duration:    duration,
		CreatedWith: CreatedWith,
	}
	return timeEntry
}

func (tc *TimeEntriesClient) Start(timeEntry *TimeEntry) (*TimeEntry, error) {
	data, err := json.Marshal(timeEntry)
	if err != nil {
		return nil, err
	}
	rawMessage, err := tc.config.SendRequest("POST", fmt.Sprintf("workspaces/%d/time_entries", timeEntry.WorkspaceID), bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(*rawMessage, timeEntry)
	if err != nil {
		return nil, err
	}

	return timeEntry, nil
}

func (tc *TimeEntriesClient) Current() (*TimeEntry, error) {
	rawMessage, err := tc.config.SendRequest("GET", "me/time_entries/current", nil)
	if err != nil {
		return nil, err
	}
	var timeEntry TimeEntry
	err = json.Unmarshal(*rawMessage, &timeEntry)
	if err != nil {
		return nil, err
	}

	return &timeEntry, nil
}

func (tc *TimeEntriesClient) MostRecent() ([]TimeEntry, error) {
	rawMessage, err := tc.config.SendRequest("GET", fmt.Sprintf("me/time_entries"), nil)
	var timeEntries []TimeEntry
	err = json.Unmarshal(*rawMessage, &timeEntries)

	if err != nil {
		return nil, err
	}
	if len(timeEntries) > 0 {
		return timeEntries, nil
	}
	return nil, nil
}

func (tc *TimeEntriesClient) Create(timeEntry *TimeEntry) (*TimeEntry, error) {
	data, err := json.Marshal(timeEntry)

	if err != nil {
		return nil, err
	}

	reader := bytes.NewReader(data)
	rawMessage, err := tc.config.SendRequest("POST", fmt.Sprintf("workspaces/%d/time_entries", timeEntry.WorkspaceID), reader)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(*rawMessage, &timeEntry)
	if err != nil {
		return nil, err
	}

	return timeEntry, nil
}

func (tc *TimeEntriesClient) StopCurrent() (*TimeEntry, error) {
	timeEntry, err := tc.Current()
	if err != nil {
		return nil, err
	}
	return tc.Stop(timeEntry)
}

func (tc *TimeEntriesClient) Stop(timeEntry *TimeEntry) (*TimeEntry, error) {
	rawMessage, err := tc.config.SendRequest("PATCH", fmt.Sprintf("workspaces/%d/time_entries/%d/stop", timeEntry.WorkspaceID, timeEntry.Id), nil)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(*rawMessage, &timeEntry)
	if err != nil {
		return nil, err
	}

	return timeEntry, nil

}
