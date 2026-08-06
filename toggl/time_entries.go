package toggl

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fefeme/workingon/util"
	"math"
	"math/rand"
	"net/url"
	"time"
)

const Endpoint = "time_entries"
const CreatedWith = "working_on"

type TimeEntryList struct {
	Count       int
	TimeEntries []TimeEntry
}

type TimeEntries struct {
	client *Client
}

func (t *TimeEntry) String() string {
	return fmt.Sprintf("%s (%d) from %s to %s for %s", t.Description, t.ProjectId, t.Start,
		t.Stop, time.Duration(t.Duration)*time.Second)
}

func (t *TimeEntry) Format(dfLayout string, loc *time.Location) string {
	start := ""
	if t.Start != nil {
		start = t.Start.In(loc).Format(dfLayout)
	}

	d := t.Duration
	if d < 0 {
		return fmt.Sprintf("%s (%d) at %s (running)", t.Description, t.ProjectId, start)
	}

	return fmt.Sprintf("\"%s\" %s for %s (%d)", t.Description, start,
		time.Duration(d)*time.Second, t.ProjectId)
}

// IsRunning reports whether the entry is an open timer.
func (t *TimeEntry) IsRunning() bool {
	return t.Duration < 0
}

func (t *TimeEntry) Fuzz() {
	sig := [2]int{-1, 1}[rand.Intn(2)]
	fuzzyTime := t.Start.Add(time.Duration(rand.Intn(180)*sig) * time.Second)

	t.Start = &fuzzyTime
}

func (t *TimeEntry) Validate() error {
	if t.Duration == 0 {
		if t.Start == nil || t.Start.IsZero() {
			return fmt.Errorf("no start time given, unable to calculate duration")
		}
		if t.Stop == nil {
			return fmt.Errorf("no stop time given, unable to calculate duration")
		}
		duration := t.Stop.Sub(*t.Start)
		if math.Abs(duration.Hours()) > 999 {
			return fmt.Errorf("something went wrong - duration is more than 999 hours")
		}
		t.Duration = duration.Milliseconds() / 1000
	}
	if t.Start == nil || t.Start.IsZero() {
		return fmt.Errorf("something went wrong - no start time set")
	}
	if t.WorkspaceId == 0 {
		return fmt.Errorf("no workspace id set - api v9 requires one to create a time entry")
	}

	t.Stop = util.TimeInUTC(t.Stop)
	t.Start = util.TimeInUTC(t.Start)

	return nil
}

func (t *TimeEntries) Start(timeEntry *TimeEntry) (*TimeEntry, error) {
	return t.Add(timeEntry)
}

// Add creates a time entry. v9 scopes creation to a workspace and takes the
// entry unwrapped, where v8 posted to a global endpoint with a "time_entry"
// envelope.
func (t *TimeEntries) Add(timeEntry *TimeEntry) (*TimeEntry, error) {
	if timeEntry.WorkspaceId == 0 {
		return nil, errors.New("workspace id is required to create a time entry")
	}

	message, err := t.client.NewMessage("POST",
		fmt.Sprintf("workspaces/%d/%s", timeEntry.WorkspaceId, Endpoint), timeEntry)
	if err != nil {
		return nil, err
	}

	data, err := t.client.SendRequest(message)
	if err != nil {
		return nil, err
	}

	var res TimeEntry
	if err := json.Unmarshal(*data, &res); err != nil {
		return nil, err
	}

	return &res, nil
}

func (t *TimeEntries) List(start *time.Time, end *time.Time) (*TimeEntryList, error) {
	base, err := url.Parse(fmt.Sprintf("me/%s", Endpoint))
	if err != nil {
		return nil, err
	}

	if start != nil && end != nil {
		params := url.Values{}
		params.Add("start_date", start.Format(time.RFC3339))
		params.Add("end_date", end.Format(time.RFC3339))
		base.RawQuery = params.Encode()
	}

	message, err := t.client.NewMessage("GET", base.String(), nil)
	if err != nil {
		return nil, err
	}

	data, err := t.client.SendRequest(message)
	if err != nil {
		return nil, err
	}

	var timeEntries []TimeEntry
	if err := decodeList(*data, &timeEntries); err != nil {
		return nil, err
	}

	return &TimeEntryList{
		TimeEntries: timeEntries,
		Count:       len(timeEntries),
	}, nil
}

// Current returns the running time entry, or nil if no timer is running.
func (t *TimeEntries) Current() (*TimeEntry, error) {
	message, err := t.client.NewMessage("GET", fmt.Sprintf("me/%s/current", Endpoint), nil)
	if err != nil {
		return nil, err
	}

	data, err := t.client.SendRequest(message)
	if err != nil {
		return nil, err
	}

	// v9 answers with a bare entry, or null when nothing is running.
	var entry *TimeEntry
	if err := json.Unmarshal(*data, &entry); err != nil {
		return nil, err
	}

	if entry == nil || entry.Id == 0 {
		return nil, nil
	}
	return entry, nil
}

// MostRecent returns the entry with the latest start time.
//
// It scans rather than indexing the last element: v9 does not document the sort
// order of the listing, so relying on it would be a coin flip.
func (t *TimeEntries) MostRecent() (*TimeEntry, error) {
	timeEntries, err := t.List(nil, nil)
	if err != nil {
		return nil, err
	}

	var recent *TimeEntry
	for i := range timeEntries.TimeEntries {
		entry := &timeEntries.TimeEntries[i]
		if entry.Start == nil {
			continue
		}
		if recent == nil || entry.Start.After(*recent.Start) {
			recent = entry
		}
	}

	return recent, nil
}

// Stop closes a running time entry.
func (t *TimeEntries) Stop(workspaceId int, timeEntryId int) (*TimeEntry, error) {
	message, err := t.client.NewMessage("PATCH",
		fmt.Sprintf("workspaces/%d/%s/%d/stop", workspaceId, Endpoint, timeEntryId), nil)
	if err != nil {
		return nil, err
	}

	data, err := t.client.SendRequest(message)
	if err != nil {
		return nil, err
	}

	var res TimeEntry
	if err := json.Unmarshal(*data, &res); err != nil {
		return nil, err
	}

	return &res, nil
}

func (t *TimeEntries) StopCurrent() (*TimeEntry, error) {
	timeEntry, err := t.Current()
	if err != nil {
		return nil, err
	}

	if timeEntry == nil {
		return nil, errors.New("no time entry is currently running")
	}

	// The stop route is workspace-scoped in v9; take the workspace from the
	// entry itself so this does not depend on configuration.
	return t.Stop(timeEntry.WorkspaceId, timeEntry.Id)
}
