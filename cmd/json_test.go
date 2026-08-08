package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"
)

// jsonConfig is a config in a zone with an offset, so the times in a document
// are checked to carry where the day was worked rather than being read back in
// whatever zone the tests happen to run in.
func jsonConfig(t *testing.T) *workingon.Config {
	t.Helper()

	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skipf("no timezone database: %v", err)
	}

	cfg := &workingon.Config{}
	cfg.Settings.Location = *loc

	return cfg
}

// decoded reads a document back as a map, the way a caller who knows nothing
// about the Go types would.
func decoded(t *testing.T, value any) map[string]any {
	t.Helper()

	var out bytes.Buffer
	if err := encodeTo(&out, value); err != nil {
		t.Fatalf("encodeTo: %v", err)
	}

	var back map[string]any
	if err := json.Unmarshal(out.Bytes(), &back); err != nil {
		t.Fatalf("the document does not parse: %v\n%s", err, out.String())
	}

	return back
}

func TestEntryOfDescribesAStoppedEntry(t *testing.T) {
	cfg := jsonConfig(t)

	start := time.Date(2026, time.August, 7, 15, 3, 0, 0, time.UTC)
	stop := start.Add(90 * time.Minute)

	entry := toggl.TimeEntry{
		Id:          42,
		WorkspaceId: 1562374,
		Description: "DBQ import",
		ProjectId:   188362780,
		TaskId:      87708632,
		Start:       &start,
		Stop:        &stop,
		Duration:    int64((90 * time.Minute).Seconds()),
	}

	view := entryOf(&entry, cfg, entryNames{project: "Learning Platform", task: "Front End"})

	for _, tc := range []struct{ got, want, what string }{
		{view.Start, "2026-08-07T17:03:00+02:00", "start, in the configured zone"},
		{view.Stop, "2026-08-07T18:33:00+02:00", "stop, in the configured zone"},
		{view.Project.Name, "Learning Platform", "project name"},
		{view.Task.Name, "Front End", "task name"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.what, tc.got, tc.want)
		}
	}

	if view.Seconds != 5400 {
		t.Errorf("seconds = %d, want 5400", view.Seconds)
	}
	if view.Running {
		t.Error("running = true, want false for an entry with a stop")
	}
}

// A running timer has no stop, and its length is how far it has got.
func TestEntryOfDescribesARunningEntry(t *testing.T) {
	cfg := jsonConfig(t)

	start := time.Now().Add(-30 * time.Minute)
	entry := toggl.TimeEntry{Id: 42, Description: "DBQ import", Start: &start, Duration: -1}

	view := entryOf(&entry, cfg, entryNames{})

	if !view.Running {
		t.Error("running = false, want true")
	}
	if view.Stop != "" {
		t.Errorf("stop = %q, want none at all", view.Stop)
	}

	// A window rather than an exact number, since the clock moves between
	// building the entry and reading it back.
	if view.Seconds < 1790 || view.Seconds > 1810 {
		t.Errorf("seconds = %d, want about 1800", view.Seconds)
	}
}

// The human output writes "#123" where a lookup failed. In a document the id is
// a field of its own, and a name repeating it would read as a real name.
func TestNamedDropsTheIdStandIn(t *testing.T) {
	for name, tc := range map[string]struct {
		id       int
		name     string
		wantNil  bool
		wantName string
	}{
		"a name that resolved": {id: 7, name: "Front End", wantName: "Front End"},
		"the id stand-in":      {id: 7, name: "#7", wantName: ""},
		"an id with no name":   {id: 7, wantName: ""},
		"neither":              {wantNil: true},
	} {
		t.Run(name, func(t *testing.T) {
			got := named(tc.id, tc.name)

			if tc.wantNil {
				if got != nil {
					t.Fatalf("ref = %v, want none at all", got)
				}
				return
			}

			if got == nil {
				t.Fatal("ref = nil, want the id at least")
			}
			if got.Id != tc.id || got.Name != tc.wantName {
				t.Errorf("ref = %+v, want id %d and name %q", got, tc.id, tc.wantName)
			}
		})
	}
}

// Nothing running is a fact to read, not a null to be told apart from a lookup
// that failed.
func TestCurrentSaysWhenNothingIsRunning(t *testing.T) {
	document := decoded(t, struct {
		Running bool       `json:"running"`
		Entry   *entryJSON `json:"entry"`
	}{false, entryWith(nil, jsonConfig(t))})

	if document["running"] != false {
		t.Errorf("running = %v, want false", document["running"])
	}
	if entry, present := document["entry"]; !present || entry != nil {
		t.Errorf("entry = %v, want an explicit null", entry)
	}
}

// A day's total is added up here so that every caller does not have to, and a
// running timer has to count toward it.
func TestDayTotalsTheEntriesItLists(t *testing.T) {
	cfg := jsonConfig(t)
	day := time.Date(2026, time.August, 7, 0, 0, 0, 0, time.UTC)

	first := day.Add(9 * time.Hour)
	second := day.Add(11 * time.Hour)

	entries := []toggl.TimeEntry{
		{Id: 1, Description: "one", Start: &first, Duration: 3600},
		{Id: 2, Description: "two", Start: &second, Duration: 1800},
	}

	var out bytes.Buffer
	views := make([]*entryJSON, 0, len(entries))
	var total int64
	for i := range entries {
		view := entryOf(&entries[i], cfg, entryNames{})
		total += view.Seconds
		views = append(views, view)
	}
	if err := encodeTo(&out, struct {
		Date         string       `json:"date"`
		Entries      []*entryJSON `json:"entries"`
		TotalSeconds int64        `json:"total_seconds"`
	}{day.Format("2006-01-02"), views, total}); err != nil {
		t.Fatalf("encodeTo: %v", err)
	}

	if total != 5400 {
		t.Errorf("total = %d, want 5400", total)
	}
	if !strings.Contains(out.String(), `"date": "2026-08-07"`) {
		t.Errorf("document does not carry the date:\n%s", out.String())
	}
}

// An ampersand in a project name is an ampersand, not an HTML entity.
func TestDocumentsDoNotEscapeProse(t *testing.T) {
	var out bytes.Buffer
	if err := encodeTo(&out, map[string]string{"name": "Bread & Butter"}); err != nil {
		t.Fatalf("encodeTo: %v", err)
	}

	if !strings.Contains(out.String(), "Bread & Butter") {
		t.Errorf("document escaped the prose:\n%s", out.String())
	}
}

// A failure is a document too, so that a caller parses one thing whatever
// happened.
func TestErrorsAreDocumentsAsWell(t *testing.T) {
	var out bytes.Buffer
	if err := encodeTo(&out, struct {
		Error string `json:"error"`
	}{"no time entry is currently running"}); err != nil {
		t.Fatalf("encodeTo: %v", err)
	}

	var back struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &back); err != nil {
		t.Fatalf("the error does not parse: %v", err)
	}
	if back.Error != "no time entry is currently running" {
		t.Errorf("error = %q, want the message it was given", back.Error)
	}
}

// Asking for JSON means a program is reading, and a program cannot answer a
// prompt - it would sit waiting on an answer that never comes.
func TestJSONIsNeverInteractive(t *testing.T) {
	was := jsonOutput
	defer func() { jsonOutput = was }()

	jsonOutput = true
	if interactive() {
		t.Error("interactive = true under --json, want false")
	}
}
