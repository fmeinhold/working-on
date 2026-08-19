package workingon

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fefeme/workingon/toggl"
)

func recentFrom(t *testing.T, body string) []toggl.TimeEntry {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer server.Close()

	now := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)

	entries, err := recentEntries(toggl.NewTogglAt("test-token", server.URL), now)
	if err != nil {
		t.Fatalf("recentEntries: %v", err)
	}

	return entries
}

// The same work booked five times over is one thing to pick up, not five - a
// listing of ten rows saying the same thing is no listing at all.
func TestRecentEntriesFoldsRepeatedWorkTogether(t *testing.T) {
	entries := recentFrom(t, `[
		{"id":1,"description":"DBQ import","project_id":10,"task_id":100,
		 "start":"2026-08-17T09:00:00Z","stop":"2026-08-17T10:00:00Z","duration":3600},
		{"id":2,"description":"DBQ import","project_id":10,"task_id":100,
		 "start":"2026-08-18T09:00:00Z","stop":"2026-08-18T10:00:00Z","duration":3600},
		{"id":3,"description":"Parser review","project_id":10,"task_id":100,
		 "start":"2026-08-16T09:00:00Z","stop":"2026-08-16T10:00:00Z","duration":3600}]`)

	if len(entries) != 2 {
		t.Fatalf("got %d entries, want the repeat folded away", len(entries))
	}

	// The one that survives is the most recent of them, since that is the one
	// whose project and task are still what you meant.
	if entries[0].Id != 2 {
		t.Errorf("kept entry %d, want the newer DBQ import", entries[0].Id)
	}
}

// Same words, different project: not the same work, and continuing one is not
// continuing the other.
func TestRecentEntriesKeepsTheSameWordsUnderDifferentProjects(t *testing.T) {
	entries := recentFrom(t, `[
		{"id":1,"description":"Standup","project_id":10,
		 "start":"2026-08-18T09:00:00Z","stop":"2026-08-18T09:15:00Z","duration":900},
		{"id":2,"description":"Standup","project_id":20,
		 "start":"2026-08-17T09:00:00Z","stop":"2026-08-17T09:15:00Z","duration":900}]`)

	if len(entries) != 2 {
		t.Errorf("got %d entries, want both projects kept", len(entries))
	}
}

// Newest first, whatever order the api answered in - which of two identical
// entries is kept depends on it entirely.
func TestRecentEntriesAnswersNewestFirst(t *testing.T) {
	entries := recentFrom(t, `[
		{"id":1,"description":"Oldest","start":"2026-08-01T09:00:00Z","stop":"2026-08-01T10:00:00Z","duration":3600},
		{"id":2,"description":"Newest","start":"2026-08-18T09:00:00Z","stop":"2026-08-18T10:00:00Z","duration":3600},
		{"id":3,"description":"Middle","start":"2026-08-10T09:00:00Z","stop":"2026-08-10T10:00:00Z","duration":3600}]`)

	want := []string{"Newest", "Middle", "Oldest"}
	for i, description := range want {
		if entries[i].Description != description {
			t.Errorf("entry %d is %q, want %q", i, entries[i].Description, description)
		}
	}
}

// There is nothing to continue about work that has not stopped.
func TestRecentEntriesLeavesOutARunningTimer(t *testing.T) {
	entries := recentFrom(t, `[
		{"id":1,"description":"Still going","start":"2026-08-19T09:00:00Z","duration":-1},
		{"id":2,"description":"Finished","start":"2026-08-18T09:00:00Z","stop":"2026-08-18T10:00:00Z","duration":3600}]`)

	if len(entries) != 1 || entries[0].Description != "Finished" {
		t.Errorf("got %v, want only the finished entry", entries)
	}
}

func TestRecentEntriesWithNothingTracked(t *testing.T) {
	if entries := recentFrom(t, `[]`); len(entries) != 0 {
		t.Errorf("got %d entries from an empty listing", len(entries))
	}
}

// Continuing something chosen from the listing is the same act as continuing
// the last thing: a new timer that looks like the old entry, which keeps its
// own record.
func TestContinueEntryRefusesWhatIsStillRunning(t *testing.T) {
	start := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	running := &toggl.TimeEntry{Description: "Still going", Start: &start, Duration: -1}

	_, err := ContinueEntry(&Config{}, running, EntryRequest{})

	if err == nil {
		t.Fatal("a running entry was continued")
	}
	if err.Error() != `"Still going" is already running` {
		t.Errorf("error = %q, want it to say the entry is already running", err)
	}
}

func TestContinueEntryWithNothingToContinue(t *testing.T) {
	if _, err := ContinueEntry(&Config{}, nil, EntryRequest{}); err == nil {
		t.Fatal("nil was continued")
	}
}
