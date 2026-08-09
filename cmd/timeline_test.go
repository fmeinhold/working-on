package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/fefeme/workingon/toggl"
)

// at is a time of day on the day the timeline tests draw, in Berlin.
func at(hour, minute int) time.Time {
	return time.Date(2026, 8, 6, hour, minute, 0, 0, berlin())
}

func tracked(from, to time.Time, description string) toggl.TimeEntry {
	return toggl.TimeEntry{
		Description: description,
		Start:       &from,
		Stop:        &to,
		Duration:    int64(to.Sub(from).Seconds()),
	}
}

// plainTimeline renders without colour, so a test reads what a pipe would.
func plainTimeline(t *testing.T, entries []toggl.TimeEntry, window dayWindow) string {
	t.Helper()

	original := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = original })

	return RenderTimeline(at(0, 0), entries, nowConfig(t), window, noNames)
}

// rowFor is the bar a slot was drawn as, between its two frame characters.
func rowFor(out, slot string) string {
	prefix := " " + slot + " │"

	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		bar, _, _ := strings.Cut(strings.TrimPrefix(line, prefix), "│")
		return bar
	}

	return ""
}

func TestRenderTimelineDrawsTheWorkingDayByDefault(t *testing.T) {
	out := plainTimeline(t, []toggl.TimeEntry{
		tracked(at(9, 0), at(10, 0), "Something"),
	}, defaultWindow)

	for _, want := range []string{" 06:00 │", " 17:30 │"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing the %q row:\n%s", want, out)
		}
	}

	// 18:00 is where the window ends, so it is the first slot not drawn.
	for _, unwanted := range []string{" 05:30 │", " 18:00 │"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("output has a %q row outside the window:\n%s", unwanted, out)
		}
	}
}

// The window says what a day is expected to look like; it must not hide work.
func TestRenderTimelineWidensForWhatFallsOutsideTheWindow(t *testing.T) {
	out := plainTimeline(t, []toggl.TimeEntry{
		tracked(at(5, 20), at(6, 0), "Early start"),
		tracked(at(18, 40), at(19, 10), "Late finish"),
	}, defaultWindow)

	for _, want := range []string{" 05:00 │", " 19:00 │", "Early start", "Late finish"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, " 19:30 │") {
		t.Errorf("timeline runs past the last entry:\n%s", out)
	}
}

// An end that was asked for is a bound, not a suggestion - but what it cuts off
// has to be accounted for, or the total would disagree with the bars.
func TestRenderTimelineHoldsAnEndThatWasAskedFor(t *testing.T) {
	window := defaultWindow
	window.from, window.fixedFrom = clockTime{hour: 9}, true

	out := plainTimeline(t, []toggl.TimeEntry{
		tracked(at(7, 0), at(8, 0), "Before"),
		tracked(at(9, 0), at(10, 0), "Inside"),
	}, window)

	if strings.Contains(out, " 07:00 │") {
		t.Errorf("--from was widened past the hour it was given:\n%s", out)
	}
	if !strings.Contains(out, "1h outside 09:00-18:00 is not drawn, across 1 entry.") {
		t.Errorf("output does not account for the entry it left out:\n%s", out)
	}
	if !strings.Contains(out, "Total   2h") {
		t.Errorf("the total no longer covers the whole day:\n%s", out)
	}
}

// An entry the window only clips counts for the part that was not drawn.
func TestRenderTimelineAccountsForAClippedEntry(t *testing.T) {
	window := defaultWindow
	window.to, window.fixedTo = clockTime{hour: 10}, true

	out := plainTimeline(t, []toggl.TimeEntry{
		tracked(at(9, 0), at(10, 30), "Runs past the end"),
	}, window)

	if !strings.Contains(out, "30m outside 06:00-10:00 is not drawn, across 1 entry.") {
		t.Errorf("output does not account for the half hour it clipped:\n%s", out)
	}
}

func TestRenderTimelineFillsTheTimeAnEntryCovers(t *testing.T) {
	out := plainTimeline(t, []toggl.TimeEntry{
		tracked(at(7, 15), at(8, 0), "Something"),
	}, defaultWindow)

	cases := []struct{ slot, want string }{
		{"06:30", "            "},
		{"07:00", "      ██████"},
		{"07:30", "████████████"},
		{"08:00", "            "},
	}

	for _, tc := range cases {
		if got := rowFor(out, tc.slot); got != tc.want {
			t.Errorf("%s row is %q, want %q\n%s", tc.slot, got, tc.want, out)
		}
	}
}

// Two entries back to back leave no gap between their blocks, and the moment
// one hands over to the other is where the second one's label starts.
func TestRenderTimelineDrawsEntriesBackToBack(t *testing.T) {
	out := plainTimeline(t, []toggl.TimeEntry{
		tracked(at(9, 0), at(9, 20), "First"),
		tracked(at(9, 20), at(9, 30), "Second"),
	}, defaultWindow)

	if got := rowFor(out, "09:00"); got != "████████████" {
		t.Errorf("09:00 row is %q, want it filled\n%s", got, out)
	}

	row := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, " 09:00 │") {
			row = line
		}
	}
	if !strings.Contains(row, "First") {
		t.Errorf("the 09:00 row is not labelled with the entry that starts it:\n%s", out)
	}
	if !strings.Contains(out, "Second") {
		t.Errorf("the second entry of the slot is unlabelled:\n%s", out)
	}
}

func TestRenderTimelineLeavesAGapWhereNothingWasTracked(t *testing.T) {
	out := plainTimeline(t, []toggl.TimeEntry{
		tracked(at(9, 0), at(9, 10), "First"),
		tracked(at(9, 20), at(9, 30), "Second"),
	}, defaultWindow)

	if got := rowFor(out, "09:00"); got != "████    ████" {
		t.Errorf("09:00 row is %q, want the untracked ten minutes left blank\n%s", got, out)
	}
}

func TestRenderTimelineCountsTheRunningEntry(t *testing.T) {
	start := time.Now().In(berlin()).Add(-30 * time.Minute)
	entries := []toggl.TimeEntry{{
		Description: "Still going",
		Start:       &start,
		Duration:    toggl.RunningDuration,
	}}

	original := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = original })

	cfg := nowConfig(t)
	out := RenderTimeline(startOfDay(start, &cfg.Settings.Location), entries, cfg,
		defaultWindow, noNames)

	for _, want := range []string{"Still going", "running", "Total"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderTimelineTotalsTheDay(t *testing.T) {
	out := plainTimeline(t, []toggl.TimeEntry{
		tracked(at(9, 0), at(10, 30), "First"),
		tracked(at(11, 0), at(11, 25), "Second"),
	}, defaultWindow)

	if !strings.Contains(out, "Total   1h 55m") {
		t.Errorf("output missing the day's total:\n%s", out)
	}
}

func TestRenderTimelineWithoutEntries(t *testing.T) {
	out := plainTimeline(t, nil, defaultWindow)

	if !strings.Contains(out, "Nothing tracked on Thursday, 6.8.2026") {
		t.Errorf("expected an empty day message, got:\n%s", out)
	}
}

// A label repeating the description as its project or task says nothing twice.
func TestTimelineLabelLeavesOutWhatItWouldRepeat(t *testing.T) {
	entry := tracked(at(9, 0), at(10, 0), "05 Front End Development")
	names := entryNames{project: "Learning Platform", task: "05 Front End Development"}

	got := timelineLabel(&entry, names, time.Hour)
	want := "05 Front End Development · Learning Platform (1h)"

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseClockFlag(t *testing.T) {
	cases := []struct {
		in   string
		want clockTime
	}{
		{"6", clockTime{hour: 6}},
		{"06:00", clockTime{hour: 6}},
		{"18:30", clockTime{hour: 18, minute: 30}},
		{"24", clockTime{hour: 24}},
	}

	for _, tc := range cases {
		got, err := parseClockFlag("from", tc.in)
		if err != nil {
			t.Errorf("parseClockFlag(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseClockFlag(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}

	for _, bad := range []string{"25", "9:70", "lunchtime"} {
		if _, err := parseClockFlag("to", bad); err == nil {
			t.Errorf("parseClockFlag(%q) was accepted", bad)
		}
	}
}

func TestParseWindow(t *testing.T) {
	window, err := parseWindow("8", "20:30")
	if err != nil {
		t.Fatalf("parseWindow: %v", err)
	}
	if window.from != (clockTime{hour: 8}) || window.to != (clockTime{hour: 20, minute: 30}) {
		t.Errorf("got %v", window)
	}

	if _, err := parseWindow("", ""); err != nil {
		t.Errorf("the default window was rejected: %v", err)
	}

	if _, err := parseWindow("18", "9"); err == nil {
		t.Error("a window that ends before it starts was accepted")
	}
}
