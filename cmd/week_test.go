package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/fefeme/workingon/toggl"
)

// The reference week: Monday 17 August 2026 to Sunday the 23rd.
func monday(loc *time.Location) time.Time {
	return time.Date(2026, time.August, 17, 0, 0, 0, 0, loc)
}

func on(loc *time.Location, day, hour, minute int, length time.Duration,
	description string, projectId int) toggl.TimeEntry {

	start := time.Date(2026, time.August, day, hour, minute, 0, 0, loc).UTC()
	stop := start.Add(length)

	return toggl.TimeEntry{
		Id: day*100 + hour, WorkspaceId: 1562374, Description: description,
		ProjectId: projectId, Start: &start, Stop: &stop,
		Duration: int64(length.Seconds()),
	}
}

// namedProjects answers for the entries above without going near the network.
func namedProjects(entry *toggl.TimeEntry) entryNames {
	switch entry.ProjectId {
	case 1:
		return entryNames{project: "Learning Platform"}
	case 2:
		return entryNames{project: "First Aid"}
	}
	return entryNames{}
}

// Whichever day you ask about, the week it belongs to is the one that began on
// the Monday - including the Sunday, which ends that week rather than opening
// the next.
func TestWeekStartIsTheFirstDayOfThatWeek(t *testing.T) {
	loc := time.UTC
	want := monday(loc)

	for day := 17; day <= 23; day++ {
		asked := time.Date(2026, time.August, day, 14, 30, 0, 0, loc)
		if got := weekStart(asked, loc, time.Monday); !got.Equal(want) {
			t.Errorf("week of the %dth starts %s, want %s", day, got, want)
		}
	}
}

// The clocks going back make one day of that week 25 hours long, which is why
// the days are counted as dates rather than as hours since the Monday.
func TestWeekStartSurvivesTheClocksChanging(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skipf("no timezone database: %v", err)
	}

	// Central European Summer Time ended on Sunday 25 October 2026.
	sunday := time.Date(2026, time.October, 25, 12, 0, 0, 0, loc)
	want := time.Date(2026, time.October, 19, 0, 0, 0, 0, loc)

	got := weekStart(sunday, loc, time.Monday)
	if !got.Equal(want) {
		t.Errorf("week starts %s, want the Monday %s", got, want)
	}
	if hour, minute, _ := got.Clock(); hour != 0 || minute != 0 {
		t.Errorf("week starts at %02d:%02d, want midnight", hour, minute)
	}
}

func TestWeekOfPlacesEntriesOnTheDayTheyBeganOn(t *testing.T) {
	loc := time.UTC

	week := weekOf(monday(loc), []toggl.TimeEntry{
		on(loc, 17, 9, 0, 2*time.Hour, "parser", 1),
		on(loc, 17, 13, 0, 90*time.Minute, "review", 2),
		on(loc, 20, 10, 0, time.Hour, "import", 1),
	}, namedProjects)

	if len(week) != 7 {
		t.Fatalf("week has %d days, want seven", len(week))
	}

	if week[0].Entries != 2 || week[0].Tracked != 3*time.Hour+30*time.Minute {
		t.Errorf("Monday = %d entries, %s; want 2 and 3h30m", week[0].Entries, week[0].Tracked)
	}
	if week[3].Entries != 1 || week[3].Tracked != time.Hour {
		t.Errorf("Thursday = %d entries, %s; want 1 and 1h", week[3].Entries, week[3].Tracked)
	}
	if week[6].Entries != 0 {
		t.Errorf("Sunday = %d entries, want none", week[6].Entries)
	}
}

// The api is asked for a range, not trusted to answer within one, and an entry
// from the week before is not this week's to count.
func TestWeekOfLeavesOutWhatBeganOutsideTheWeek(t *testing.T) {
	loc := time.UTC

	week := weekOf(monday(loc), []toggl.TimeEntry{
		on(loc, 16, 9, 0, 2*time.Hour, "the Sunday before", 1),
		on(loc, 24, 9, 0, 2*time.Hour, "the Monday after", 1),
		on(loc, 19, 9, 0, time.Hour, "this week", 1),
	}, namedProjects)

	var total time.Duration
	for _, day := range week {
		total += day.Tracked
	}

	if total != time.Hour {
		t.Errorf("total = %s, want only the hour inside the week", total)
	}
}

// The projects are what a day was spent on, each named once, in the order the
// day met them.
func TestWeekOfNamesEachProjectOnceInOrder(t *testing.T) {
	loc := time.UTC

	week := weekOf(monday(loc), []toggl.TimeEntry{
		on(loc, 17, 9, 0, time.Hour, "first", 2),
		on(loc, 17, 11, 0, time.Hour, "second", 1),
		on(loc, 17, 14, 0, time.Hour, "third", 2),
		on(loc, 17, 16, 0, time.Hour, "no project", 0),
	}, namedProjects)

	want := []string{"First Aid", "Learning Platform"}
	if strings.Join(week[0].Projects, "|") != strings.Join(want, "|") {
		t.Errorf("projects = %v, want %v", week[0].Projects, want)
	}
}

func TestWeekOfMarksADayWithATimerStillRunning(t *testing.T) {
	loc := time.UTC

	running := on(loc, 19, 9, 0, 0, "still going", 1)
	running.Stop = nil
	running.Duration = toggl.RunningDuration

	week := weekOf(monday(loc), []toggl.TimeEntry{running}, namedProjects)

	if !week[2].Running {
		t.Error("Wednesday does not know its timer is still running")
	}
}

func TestRenderWeekShowsEveryDayAndTheWeeksHours(t *testing.T) {
	loc := time.UTC

	week := weekOf(monday(loc), []toggl.TimeEntry{
		on(loc, 17, 9, 0, 2*time.Hour, "parser", 1),
		on(loc, 20, 10, 0, 90*time.Minute, "import", 2),
	}, namedProjects)

	out := RenderWeek(week, sanitizeConfig())

	for _, day := range []string{"Monday", "Tuesday", "Wednesday", "Thursday",
		"Friday", "Saturday", "Sunday"} {
		if !strings.Contains(out, day) {
			t.Errorf("%s is missing from the week:\n%s", day, out)
		}
	}

	if !strings.Contains(out, "17.8.2026 to 23.8.2026") {
		t.Errorf("the week it covers is not in the heading:\n%s", out)
	}
	if !strings.Contains(out, "3h 30m") {
		t.Errorf("the week's hours are not totalled:\n%s", out)
	}
	if !strings.Contains(out, "Learning Platform") {
		t.Errorf("the projects a day was spent on are missing:\n%s", out)
	}
}

// A day nobody worked is a row too - a blank Wednesday is something you want to
// see - and it reads as a dash rather than as a measured nought.
func TestRenderWeekWritesADayWithNothingOnItAsADash(t *testing.T) {
	loc := time.UTC

	week := weekOf(monday(loc), []toggl.TimeEntry{
		on(loc, 17, 9, 0, time.Hour, "parser", 1),
	}, namedProjects)

	out := RenderWeek(week, sanitizeConfig())

	saturday := lineWith(out, "Saturday")
	if !strings.Contains(saturday, "-") || strings.Contains(saturday, "0m") {
		t.Errorf("Saturday = %q, want dashes rather than a nought", saturday)
	}
}

func TestRenderWeekSaysWhenNothingWasTrackedAtAll(t *testing.T) {
	week := weekOf(monday(time.UTC), nil, namedProjects)

	out := RenderWeek(week, sanitizeConfig())

	if !strings.Contains(out, "Nothing tracked between 17.8.2026 and 23.8.2026") {
		t.Errorf("output = %q, want it to say the week was empty", out)
	}
}

func TestRenderWeekSaysTheTotalIsStillMoving(t *testing.T) {
	loc := time.UTC

	running := on(loc, 19, 9, 0, 0, "still going", 1)
	running.Stop = nil
	running.Duration = toggl.RunningDuration

	out := RenderWeek(weekOf(monday(loc), []toggl.TimeEntry{running}, namedProjects),
		sanitizeConfig())

	if !strings.Contains(out, "A timer is still running") {
		t.Errorf("output does not say the total is still moving:\n%s", out)
	}
}

// A workspace that files nothing under a project is not asked to read an empty
// column all week.
func TestRenderWeekLeavesOutTheProjectsColumnWhereThereAreNone(t *testing.T) {
	loc := time.UTC

	week := weekOf(monday(loc), []toggl.TimeEntry{
		on(loc, 17, 9, 0, time.Hour, "parser", 0),
	}, namedProjects)

	if out := RenderWeek(week, sanitizeConfig()); strings.Contains(out, "Projects") {
		t.Errorf("output carries an empty projects column:\n%s", out)
	}
}

func lineWith(text, needle string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

// Where a week begins is a thing countries and the people in them disagree
// about, so it is asked rather than assumed.
func TestWeekStartCountsBackToTheConfiguredDay(t *testing.T) {
	loc := time.UTC

	for name, tc := range map[string]struct {
		starts time.Weekday
		asked  int
		want   int
	}{
		"a Sunday week from its own Sunday":  {time.Sunday, 23, 23},
		"a Sunday week from the Wednesday":   {time.Sunday, 19, 16},
		"a Sunday week from the Saturday":    {time.Sunday, 22, 16},
		"a Monday week from the Sunday":      {time.Monday, 23, 17},
		"a Saturday week from the Wednesday": {time.Saturday, 19, 15},
	} {
		t.Run(name, func(t *testing.T) {
			asked := time.Date(2026, time.August, tc.asked, 14, 30, 0, 0, loc)
			want := time.Date(2026, time.August, tc.want, 0, 0, 0, 0, loc)

			if got := weekStart(asked, loc, tc.starts); !got.Equal(want) {
				t.Errorf("week starts %s, want %s", got, want)
			}
		})
	}
}

func TestParseWeekStartReadsAWeekdayName(t *testing.T) {
	for value, want := range map[string]time.Weekday{
		"":         time.Monday,
		"monday":   time.Monday,
		"Sunday":   time.Sunday,
		"sun":      time.Sunday,
		"  SAT  ":  time.Saturday,
		"saturday": time.Saturday,
	} {
		t.Run(value, func(t *testing.T) {
			got, err := parseWeekStart(value)
			if err != nil {
				t.Fatalf("parseWeekStart(%q): %v", value, err)
			}
			if got != want {
				t.Errorf("week starts %s, want %s", got, want)
			}
		})
	}
}

func TestParseWeekStartRefusesWhatIsNotADay(t *testing.T) {
	if _, err := parseWeekStart("funday"); err == nil {
		t.Error("read \"funday\" as a day of the week")
	}
}

// A week read from Sunday is still seven days, and still ends the day before
// it began.
func TestWeekOfFollowsTheConfiguredStart(t *testing.T) {
	loc := time.UTC
	start := weekStart(time.Date(2026, time.August, 19, 9, 0, 0, 0, loc), loc, time.Sunday)

	week := weekOf(start, []toggl.TimeEntry{
		on(loc, 16, 9, 0, time.Hour, "the Sunday", 1),
		on(loc, 22, 9, 0, time.Hour, "the Saturday", 1),
		on(loc, 23, 9, 0, time.Hour, "the Sunday after", 1),
	}, namedProjects)

	if week[0].Day.Weekday() != time.Sunday {
		t.Errorf("week opens on %s, want Sunday", week[0].Day.Weekday())
	}
	if week[0].Entries != 1 || week[6].Entries != 1 {
		t.Errorf("week runs %d to %d entries, want one at each end",
			week[0].Entries, week[6].Entries)
	}

	var total time.Duration
	for _, day := range week {
		total += day.Tracked
	}
	if total != 2*time.Hour {
		t.Errorf("total = %s, want the Sunday after left out", total)
	}
}
