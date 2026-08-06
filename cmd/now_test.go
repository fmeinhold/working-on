package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"
)

func nowConfig(t *testing.T) *workingon.Config {
	t.Helper()

	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("loading location: %v", err)
	}

	cfg := &workingon.Config{}
	cfg.Settings.Location = *loc
	cfg.Settings.DateLayout = "2.1.2006"
	cfg.Settings.DateTimeLayout = "2.1.2006 15:04"

	return cfg
}

// withNames substitutes the project and task lookup for the duration of a test.
func withNames(t *testing.T, names entryNames) {
	t.Helper()

	original := nameResolver
	nameResolver = func(*toggl.TimeEntry) entryNames { return names }
	t.Cleanup(func() { nameResolver = original })
}

func TestRenderCurrentLaysOutTheWholeEntry(t *testing.T) {
	cfg := nowConfig(t)
	withNames(t, entryNames{project: "Internal Tools", task: "Toggl v9 port"})

	start := time.Date(2026, 8, 6, 12, 16, 0, 0, time.UTC)
	entry := &toggl.TimeEntry{
		Description: "Refactoring the arg parser",
		ProjectId:   12345678,
		TaskId:      87654321,
		Start:       &start,
		Duration:    toggl.RunningDuration,
	}

	got := RenderCurrent(entry, cfg, false)

	// Elapsed is relative to the clock, so only the fixed rows are compared.
	head, _, _ := strings.Cut(got, "   Elapsed")
	want := "⏲  Currently working on\n\n" +
		"   Description   Refactoring the arg parser\n" +
		"   Project       Internal Tools\n" +
		"   Task          Toggl v9 port\n" +
		"   Started       Thursday, 6.8.2026 14:16\n"

	if head != want {
		t.Errorf("got:\n%s\nwant:\n%s", head, want)
	}
}

func TestRenderCurrentShowsDescriptionAndLocalStart(t *testing.T) {
	cfg := nowConfig(t)

	// 12:16 UTC is 14:16 in Berlin; the entry must be reported in the second.
	start := time.Date(2026, 8, 6, 12, 16, 0, 0, time.UTC)
	entry := &toggl.TimeEntry{
		Description: "Refactoring the arg parser",
		Start:       &start,
		Duration:    toggl.RunningDuration,
	}

	out := RenderCurrent(entry, cfg, false)

	for _, want := range []string{
		"Refactoring the arg parser",
		"Thursday, 6.8.2026 14:16",
		"Elapsed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderCurrentOmitsFieldsItCannotFill(t *testing.T) {
	cfg := nowConfig(t)

	start := time.Now().Add(-90 * time.Minute)
	entry := &toggl.TimeEntry{Start: &start, Duration: toggl.RunningDuration}

	out := RenderCurrent(entry, cfg, false)

	for _, unwanted := range []string{"Description", "Project", "Task"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("output has an empty %q row:\n%s", unwanted, out)
		}
	}
	if !strings.Contains(out, "1h 30m") {
		t.Errorf("output missing the elapsed time:\n%s", out)
	}
}

func TestRenderCurrentWithoutAnEntry(t *testing.T) {
	out := RenderCurrent(nil, nowConfig(t), false)

	if !strings.Contains(out, "Nothing is running") {
		t.Errorf("expected an idle message, got:\n%s", out)
	}
}

func TestRenderCurrentPromptStaysTerse(t *testing.T) {
	cfg := nowConfig(t)
	start := time.Now()
	entry := &toggl.TimeEntry{Description: "Something", Start: &start}

	for _, e := range []*toggl.TimeEntry{entry, nil} {
		out := RenderCurrent(e, cfg, true)
		if strings.Contains(out, "Something") || strings.Contains(out, "\n\n") {
			t.Errorf("prompt output is not terse: %q", out)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{30 * time.Second, "less than a minute"},
		{time.Minute + 20*time.Second, "1m"},
		{45 * time.Minute, "45m"},
		{2 * time.Hour, "2h"},
		{2*time.Hour + 31*time.Minute + 12*time.Second, "2h 31m"},
	}

	for _, tc := range cases {
		if got := humanDuration(tc.in); got != tc.want {
			t.Errorf("humanDuration(%s) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A layout that already names the weekday must not have a second one bolted on.
func TestFormatMomentRespectsAWeekdayInTheLayout(t *testing.T) {
	moment := time.Date(2026, 8, 6, 14, 16, 0, 0, time.UTC)

	if got := formatMoment(moment, "Mon 2.1.2006 15:04"); got != "Thu 6.8.2026 14:16" {
		t.Errorf("got %q", got)
	}
}
