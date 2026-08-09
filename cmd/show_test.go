package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/fefeme/workingon/toggl"
)

func finished(start time.Time, minutes int, description string) toggl.TimeEntry {
	stop := start.Add(time.Duration(minutes) * time.Minute)
	return toggl.TimeEntry{
		Description: description,
		Start:       &start,
		Stop:        &stop,
		Duration:    int64(minutes) * 60,
	}
}

// noNames is the resolver for tests that do not care where an entry was filed.
func noNames(*toggl.TimeEntry) entryNames { return entryNames{} }

func TestRenderDayListsEntriesWithATotal(t *testing.T) {
	cfg := nowConfig(t)

	day := time.Date(2026, 8, 6, 0, 0, 0, 0, &cfg.Settings.Location)
	entries := []toggl.TimeEntry{
		// 07:00 UTC is 09:00 in Berlin, which is where it must be reported.
		finished(time.Date(2026, 8, 6, 7, 0, 0, 0, time.UTC), 90, "Refactoring the arg parser"),
		finished(time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC), 25, "Writing the show command"),
	}

	out := RenderDay(day, entries, cfg, noNames)

	for _, want := range []string{
		"Thursday, 6.8.2026",
		"09:00", "10:30", "1h 30m", "Refactoring the arg parser",
		"11:00", "11:25", "25m", "Writing the show command",
		"Total", "1h 55m",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderDayNamesTheProjectAndTask(t *testing.T) {
	cfg := nowConfig(t)

	day := time.Date(2026, 8, 6, 0, 0, 0, 0, &cfg.Settings.Location)
	entries := []toggl.TimeEntry{
		finished(time.Date(2026, 8, 6, 7, 0, 0, 0, time.UTC), 60, "Something"),
	}
	resolve := func(*toggl.TimeEntry) entryNames {
		return entryNames{project: "Internal Tools", task: "Toggl v9 port"}
	}

	out := RenderDay(day, entries, cfg, resolve)

	for _, want := range []string{"Project", "Internal Tools", "Task", "Toggl v9 port"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// A column nothing fills is width spent on nothing.
func TestRenderDayOmitsColumnsNothingFills(t *testing.T) {
	cfg := nowConfig(t)

	day := time.Date(2026, 8, 6, 0, 0, 0, 0, &cfg.Settings.Location)
	entries := []toggl.TimeEntry{
		finished(time.Date(2026, 8, 6, 7, 0, 0, 0, time.UTC), 60, "Something"),
	}
	resolve := func(*toggl.TimeEntry) entryNames {
		return entryNames{project: "Internal Tools"}
	}

	out := RenderDay(day, entries, cfg, resolve)

	if !strings.Contains(out, "Project") {
		t.Errorf("output missing the project column:\n%s", out)
	}
	if strings.Contains(out, "Task") {
		t.Errorf("output has a task column with nothing in it:\n%s", out)
	}
}

func TestRenderDayCountsTheRunningEntry(t *testing.T) {
	cfg := nowConfig(t)

	start := time.Now().Add(-30 * time.Minute)
	day := startOfDay(start, &cfg.Settings.Location)
	entries := []toggl.TimeEntry{{
		Description: "Still going",
		Start:       &start,
		Duration:    toggl.RunningDuration,
	}}

	out := RenderDay(day, entries, cfg, noNames)

	for _, want := range []string{"running", "30m", "still running"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderDayWithoutEntries(t *testing.T) {
	cfg := nowConfig(t)
	day := time.Date(2026, 8, 6, 0, 0, 0, 0, &cfg.Settings.Location)

	out := RenderDay(day, nil, cfg, noNames)

	if !strings.Contains(out, "Nothing tracked on Thursday, 6.8.2026") {
		t.Errorf("expected an empty day message, got:\n%s", out)
	}
}

func TestRenderDayNamesAnEntryThatHasNoDescription(t *testing.T) {
	cfg := nowConfig(t)

	day := time.Date(2026, 8, 6, 0, 0, 0, 0, &cfg.Settings.Location)
	entries := []toggl.TimeEntry{
		finished(time.Date(2026, 8, 6, 7, 0, 0, 0, time.UTC), 60, ""),
	}

	if out := RenderDay(day, entries, cfg, noNames); !strings.Contains(out, "(no description)") {
		t.Errorf("an entry without a description left an empty row:\n%s", out)
	}
}

func TestEntriesStartingOnKeepsTheDayInOrder(t *testing.T) {
	loc := berlin()
	day := time.Date(2026, 8, 6, 0, 0, 0, 0, loc)

	later := finished(time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC), 30, "second")
	earlier := finished(time.Date(2026, 8, 6, 7, 0, 0, 0, time.UTC), 30, "first")
	// 22:30 UTC is half past midnight in Berlin, so this one belongs to the 7th.
	nextDay := finished(time.Date(2026, 8, 6, 22, 30, 0, 0, time.UTC), 30, "tomorrow")
	// And 22:30 UTC on the 5th is half past midnight on the 6th.
	afterMidnight := finished(time.Date(2026, 8, 5, 22, 30, 0, 0, time.UTC), 30, "late night")
	undated := toggl.TimeEntry{Description: "no start"}

	kept := entriesStartingOn(day, []toggl.TimeEntry{later, nextDay, earlier, afterMidnight, undated})

	var got []string
	for _, entry := range kept {
		got = append(got, entry.Description)
	}

	want := []string{"late night", "first", "second"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEntryEndDatesAnEntryThatHasNoStop(t *testing.T) {
	loc := berlin()
	start := time.Date(2026, 8, 6, 7, 0, 0, 0, time.UTC)
	entry := toggl.TimeEntry{Start: &start, Duration: 45 * 60}

	if got := entryEnd(&entry, "15:04", loc); got != "09:45" {
		t.Errorf("got %q, want %q", got, "09:45")
	}
}

func TestTimeLayoutIsTheTimeHalfOfTheConfiguredLayout(t *testing.T) {
	cfg := nowConfig(t)

	if got := timeLayout(cfg); got != "15:04" {
		t.Errorf("got %q, want %q", got, "15:04")
	}

	cfg.Settings.DateLayout = "2006-01-02"
	cfg.Settings.DateTimeLayout = "2006-01-02 3:04PM"
	if got := timeLayout(cfg); got != "3:04PM" {
		t.Errorf("got %q, want %q", got, "3:04PM")
	}

	// A date and time layout that is not the date layout plus a time falls back
	// rather than printing the whole thing on every row.
	cfg.Settings.DateTimeLayout = "Mon Jan 2 15:04:05 2006"
	if got := timeLayout(cfg); got != "15:04" {
		t.Errorf("got %q, want %q", got, "15:04")
	}
}
