package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"
)

func modifyConfig() *workingon.Config {
	cfg := &workingon.Config{}
	cfg.Settings.Location = *time.UTC
	cfg.Settings.DateLayout = "2.1.2006"
	cfg.Settings.DateTimeLayout = "2.1.2006 15:04"
	return cfg
}

// A time on its own belongs to the entry, not to today. Modifying yesterday's
// entry with --stop 17:00 has to land on yesterday or it books an entry that
// runs for a day.
func TestParseMomentReadsATimeAgainstTheEntrysOwnDay(t *testing.T) {
	entryDay := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)

	for name, tc := range map[string]struct {
		spec string
		want time.Time
	}{
		"a bare clock keeps the entry's date": {
			spec: "17:00",
			want: time.Date(2026, 8, 17, 17, 0, 0, 0, time.UTC),
		},
		"a date and a time say both": {
			spec: "16.8 11:30",
			want: time.Date(2026, 8, 16, 11, 30, 0, 0, time.UTC),
		},
		"either order": {
			spec: "11:30 16.8",
			want: time.Date(2026, 8, 16, 11, 30, 0, 0, time.UTC),
		},
		"a date on its own keeps the entry's time of day": {
			spec: "16.8",
			want: time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC),
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := parseMoment(modifyConfig(), tc.spec, entryDay)
			if err != nil {
				t.Fatalf("parseMoment(%q): %v", tc.spec, err)
			}
			if !got.Equal(tc.want) {
				t.Errorf("parseMoment(%q) = %s, want %s",
					tc.spec, got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
		})
	}
}

// Something that is not a time is a mistake to report. Read as a description
// it would rename the entry, which is not what --stop was asked to do.
func TestParseMomentRefusesWhatIsNotATime(t *testing.T) {
	_, err := parseMoment(modifyConfig(), "half-five", time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC))

	if err == nil {
		t.Fatal("\"half-five\" was accepted as a time")
	}
	if !strings.Contains(err.Error(), "17:00") {
		t.Errorf("error = %q, want it to show what a time looks like", err)
	}
}

// Whether a spec named a day decides what happens to a stop that falls before
// the start: one that did is taken at its word.
func TestHasDateSpotsASpecThatNamedADay(t *testing.T) {
	for spec, want := range map[string]bool{
		"17:00":          false,
		"yesterday 9:00": true,
		"16.8":           true,
		"mon 9:00":       true,
		"":               false,
	} {
		if got := hasDate(modifyConfig(), spec); got != want {
			t.Errorf("hasDate(%q) = %v, want %v", spec, got, want)
		}
	}
}

func changeOf(before, after toggl.TimeEntry, notes ...string) *workingon.Change {
	return &workingon.Change{Before: before, After: after, Notes: notes}
}

// An entry as it comes back from toggl, for the rendering to have something
// to show. The times are read from RFC 3339 so the test says what it means.
func modifiable(start string, seconds int64, project, task int) toggl.TimeEntry {
	at, _ := time.Parse(time.RFC3339, start)
	stop := at.Add(time.Duration(seconds) * time.Second)

	return toggl.TimeEntry{
		Id: 7, Description: "parser review", WorkspaceId: 1562374,
		ProjectId: project, TaskId: task,
		Start: &at, Stop: &stop, Duration: seconds,
	}
}

// Only what moved is printed. A list of every field with most of them
// unchanged buries the one line the reader is checking.
func TestRenderChangeShowsOnlyWhatMoved(t *testing.T) {
	before := modifiable("2026-08-17T09:00:00Z", 5400, 188362780, 87708632)
	after := modifiable("2026-08-17T09:00:00Z", 7200, 188362780, 87708632)

	out := renderChange(changeOf(before, after, "stop"), modifyConfig(), noNames, false)

	if !strings.Contains(out, "stop") {
		t.Errorf("output does not show the stop:\n%s", out)
	}
	if strings.Contains(out, "project") || strings.Contains(out, "task") {
		t.Errorf("output shows fields that did not change:\n%s", out)
	}
}

// A stop that landed on another day has to say so, or an entry over midnight
// reads as one that runs backwards.
func TestRenderChangeDatesAStopOnAnotherDay(t *testing.T) {
	before := modifiable("2026-08-17T22:00:00Z", 3600, 0, 0)
	after := modifiable("2026-08-17T22:00:00Z", 10800, 0, 0)

	out := renderChange(changeOf(before, after, "stop"), modifyConfig(), noNames, false)

	if !strings.Contains(out, "18.8.2026") {
		t.Errorf("output does not date the stop:\n%s", out)
	}
	if !strings.Contains(out, "23:00") {
		t.Errorf("output does not show the old stop as a clock reading:\n%s", out)
	}
}

// A dry run has to read as one. "Modified" for something that was not would be
// a lie about hours somebody worked.
func TestRenderChangeSaysWhenNothingWasSaved(t *testing.T) {
	before := modifiable("2026-08-17T09:00:00Z", 5400, 0, 0)
	after := modifiable("2026-08-17T09:00:00Z", 7200, 0, 0)

	dry := renderChange(changeOf(before, after, "stop"), modifyConfig(), noNames, true)
	if !strings.HasPrefix(dry, "Would modify") {
		t.Errorf("a dry run does not say so:\n%s", dry)
	}

	saved := renderChange(changeOf(before, after, "stop"), modifyConfig(), noNames, false)
	if !strings.HasPrefix(saved, "Modified") {
		t.Errorf("a saved change does not say so:\n%s", saved)
	}
}

// A task or project that is gone reads as "none" rather than as a zero.
func TestRenderChangeNamesWhatIsNoLongerThere(t *testing.T) {
	before := modifiable("2026-08-17T09:00:00Z", 5400, 188362780, 87708632)
	after := modifiable("2026-08-17T09:00:00Z", 5400, 178178172, 0)

	out := renderChange(changeOf(before, after, "project"), modifyConfig(),
		func(entry *toggl.TimeEntry) entryNames {
			if entry.ProjectId == 188362780 {
				return entryNames{project: "Learning Platform", task: "Front End"}
			}
			return entryNames{project: "LaunchCycle 3.0"}
		}, false)

	if !strings.Contains(out, "Learning Platform (188362780) -> LaunchCycle 3.0 (178178172)") {
		t.Errorf("output does not name both projects:\n%s", out)
	}
	if !strings.Contains(out, "-> none") {
		t.Errorf("output does not say the task is gone:\n%s", out)
	}
}

// The running timer has no stop, and saying so beats an empty column.
func TestRenderChangeSaysARunningEntryHasNoStop(t *testing.T) {
	start, _ := time.Parse(time.RFC3339, "2026-08-17T09:00:00Z")

	before := toggl.TimeEntry{Id: 8, Description: "parser review", Start: &start, Duration: -1}
	after := modifiable("2026-08-17T09:00:00Z", 5400, 0, 0)
	after.Id = 8

	out := renderChange(changeOf(before, after, "stopped"), modifyConfig(), noNames, false)

	if !strings.Contains(out, "still running ->") {
		t.Errorf("output does not say the entry had no stop:\n%s", out)
	}
	if !strings.Contains(out, "running -> 1h 30m") {
		t.Errorf("output does not show the length it came to:\n%s", out)
	}
}
