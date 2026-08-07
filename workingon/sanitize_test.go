package workingon

import (
	"testing"
	"time"

	"github.com/fefeme/workingon/toggl"
)

// berlin is a zone with a daylight saving change in it, so a grid laid from
// midnight is tested against a day that is not 24 hours long.
func berlin(t *testing.T) *time.Location {
	t.Helper()

	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skipf("no timezone database: %v", err)
	}

	return loc
}

// at is a time of day on the 7th of August 2026, in the given zone.
func at(loc *time.Location, hour, minute, second int) time.Time {
	return time.Date(2026, time.August, 7, hour, minute, second, 0, loc)
}

// tracked is an entry that ran between two times.
func tracked(description string, begin, end time.Time) toggl.TimeEntry {
	start := begin.UTC()
	stop := end.UTC()

	return toggl.TimeEntry{
		Id:          int(begin.Unix()),
		WorkspaceId: 1562374,
		Description: description,
		Start:       &start,
		Stop:        &stop,
		Duration:    int64(end.Sub(begin).Seconds()),
	}
}

// running is a timer that is still going.
func running(description string, begin time.Time) toggl.TimeEntry {
	entry := tracked(description, begin, begin)
	entry.Stop = nil
	entry.Duration = -1

	return entry
}

func tidy(loc *time.Location, zones ...Zone) Sanitizer {
	return Sanitizer{
		Snap:     DefaultSnap,
		Short:    DefaultShort,
		Zones:    zones,
		Location: loc,
		Now:      func() time.Time { return at(loc, 17, 0, 0) },
	}
}

// spanned reads an adjustment back as the clock times it puts an entry between.
func spanned(a Adjustment, loc *time.Location) string {
	return a.Start.In(loc).Format("15:04") + "-" + a.Stop.In(loc).Format("15:04")
}

func planFor(t *testing.T, s Sanitizer, entries ...toggl.TimeEntry) map[string]string {
	t.Helper()

	placed := make(map[string]string)
	for _, adjustment := range s.Plan(entries) {
		placed[adjustment.Entry.Description] = spanned(adjustment, s.location())
	}

	return placed
}

// The case this was written for: a five minute note between two long entries
// grows to meet them both.
func TestSanitizeStubTakesTheGapsAroundIt(t *testing.T) {
	loc := berlin(t)
	s := tidy(loc)

	plan := planFor(t, s,
		tracked("Standup", at(loc, 9, 0, 0), at(loc, 10, 30, 0)),
		tracked("Research into state codes", at(loc, 11, 15, 0), at(loc, 11, 20, 0)),
		tracked("Common core mappings", at(loc, 12, 0, 0), at(loc, 13, 30, 0)),
	)

	if got := plan["Research into state codes"]; got != "10:30-12:00" {
		t.Errorf("the stub runs %q, want it to meet both neighbours at 10:30-12:00", got)
	}
	if _, moved := plan["Standup"]; moved {
		t.Errorf("the entry before the stub was moved as well: %v", plan)
	}
	if _, moved := plan["Common core mappings"]; moved {
		t.Errorf("the entry after the stub was moved as well: %v", plan)
	}
}

// A gap between two entries that are both work belongs to the one that ran into
// it: that is what carrying on without saying so looks like.
func TestSanitizeGapGoesToTheEntryBeforeIt(t *testing.T) {
	loc := berlin(t)
	s := tidy(loc)

	plan := planFor(t, s,
		tracked("Standup", at(loc, 9, 0, 0), at(loc, 10, 30, 0)),
		tracked("Common core mappings", at(loc, 11, 30, 0), at(loc, 13, 0, 0)),
	)

	if got := plan["Standup"]; got != "09:00-11:30" {
		t.Errorf("the first entry runs %q, want it to close the gap at 09:00-11:30", got)
	}
	if _, moved := plan["Common core mappings"]; moved {
		t.Errorf("the later entry was moved as well: %v", plan)
	}
}

// A no work zone is a gap that stays a gap. Each side closes what it can reach
// without crossing it.
func TestSanitizeStopsAtANoWorkZone(t *testing.T) {
	loc := berlin(t)
	s := tidy(loc, Zone{FromMinute: 12 * 60, ToMinute: 13 * 60})

	plan := planFor(t, s,
		tracked("Standup", at(loc, 9, 0, 0), at(loc, 11, 20, 0)),
		tracked("Common core mappings", at(loc, 13, 30, 0), at(loc, 15, 0, 0)),
	)

	if got := plan["Standup"]; got != "09:00-12:00" {
		t.Errorf("the first entry runs %q, want it to stop at lunch, 09:00-12:00", got)
	}
	if got := plan["Common core mappings"]; got != "13:00-15:00" {
		t.Errorf("the later entry runs %q, want it to start after lunch at 13:00", got)
	}
}

// A stub is not a way around a zone either: it grows up to lunch and no further.
func TestSanitizeStubStopsAtANoWorkZone(t *testing.T) {
	loc := berlin(t)
	s := tidy(loc, Zone{FromMinute: 12 * 60, ToMinute: 13 * 60})

	plan := planFor(t, s,
		tracked("Standup", at(loc, 9, 0, 0), at(loc, 11, 0, 0)),
		tracked("Research", at(loc, 13, 30, 0), at(loc, 13, 35, 0)),
		tracked("Common core mappings", at(loc, 14, 0, 0), at(loc, 15, 0, 0)),
	)

	if got := plan["Research"]; got != "13:00-14:00" {
		t.Errorf("the stub runs %q, want it held off lunch at 13:00-14:00", got)
	}
	if got := plan["Standup"]; got != "09:00-12:00" {
		t.Errorf("the first entry runs %q, want it to stop at lunch", got)
	}
}

// An entry that overlaps a zone because that is when you worked is left alone.
func TestSanitizeLeavesWorkInsideAZoneAlone(t *testing.T) {
	loc := berlin(t)
	s := tidy(loc, Zone{FromMinute: 12 * 60, ToMinute: 13 * 60})

	plan := planFor(t, s,
		tracked("Standup", at(loc, 9, 0, 0), at(loc, 10, 0, 0)),
		tracked("Lunch and learn", at(loc, 11, 30, 0), at(loc, 12, 30, 0)),
	)

	if got := plan["Lunch and learn"]; got != "" {
		t.Errorf("the entry over lunch was moved to %q, want it left as it was", got)
	}
	if got := plan["Standup"]; got != "09:00-11:30" {
		t.Errorf("the first entry runs %q, want it to close the gap before lunch", got)
	}
}

func TestSanitizeSnapsRaggedTimes(t *testing.T) {
	loc := berlin(t)
	s := tidy(loc)

	plan := planFor(t, s,
		tracked("Standup", at(loc, 9, 3, 47), at(loc, 10, 31, 12)),
	)

	if got := plan["Standup"]; got != "09:05-10:30" {
		t.Errorf("the entry runs %q, want it rounded to 09:05-10:30", got)
	}
}

// A grid of nothing leaves the times exactly where they are.
func TestSanitizeWithoutASnapLeavesTimesAlone(t *testing.T) {
	loc := berlin(t)
	s := tidy(loc)
	s.Snap = 0

	plan := s.Plan([]toggl.TimeEntry{
		tracked("Standup", at(loc, 9, 3, 47), at(loc, 10, 31, 12)),
	})

	if len(plan) != 0 {
		t.Errorf("plan = %v, want nothing to do without a grid", plan)
	}
}

// An entry too short to survive the grid keeps its own times until the gaps
// around it give it a length.
func TestSanitizeDoesNotRoundAShortEntryAway(t *testing.T) {
	loc := berlin(t)
	s := tidy(loc)

	plan := planFor(t, s,
		tracked("Standup", at(loc, 9, 0, 0), at(loc, 10, 0, 0)),
		tracked("Note", at(loc, 11, 1, 0), at(loc, 11, 1, 30)),
		tracked("Common core mappings", at(loc, 12, 0, 0), at(loc, 13, 0, 0)),
	)

	if got := plan["Note"]; got != "10:00-12:00" {
		t.Errorf("the note runs %q, want it grown to 10:00-12:00 rather than rounded away", got)
	}
}

// A timer that is still running is not ours to move - but it still walls off
// the time before it, so the entry before it grows only as far as its start.
func TestSanitizeLeavesARunningTimerAlone(t *testing.T) {
	loc := berlin(t)
	s := tidy(loc)

	plan := planFor(t, s,
		tracked("Standup", at(loc, 9, 0, 0), at(loc, 10, 0, 0)),
		running("Common core mappings", at(loc, 11, 0, 0)),
	)

	if got := plan["Common core mappings"]; got != "" {
		t.Errorf("the running timer was moved to %q, want it left alone", got)
	}
	if got := plan["Standup"]; got != "09:00-11:00" {
		t.Errorf("the entry before it runs %q, want it grown up to the timer", got)
	}
}

// Two entries that already run into one another have nothing to tidy.
func TestSanitizeLeavesATidyDayAlone(t *testing.T) {
	loc := berlin(t)
	s := tidy(loc)

	plan := s.Plan([]toggl.TimeEntry{
		tracked("Standup", at(loc, 9, 0, 0), at(loc, 10, 30, 0)),
		tracked("Common core mappings", at(loc, 10, 30, 0), at(loc, 12, 0, 0)),
	})

	if len(plan) != 0 {
		t.Errorf("plan = %v, want nothing to do", plan)
	}
}

// Overlaps are somebody else's business: tidying must not quietly move one
// entry out from under another.
func TestSanitizeLeavesAnOverlapAlone(t *testing.T) {
	loc := berlin(t)
	s := tidy(loc)

	plan := s.Plan([]toggl.TimeEntry{
		tracked("Standup", at(loc, 9, 0, 0), at(loc, 11, 0, 0)),
		tracked("Common core mappings", at(loc, 10, 0, 0), at(loc, 12, 0, 0)),
	})

	if len(plan) != 0 {
		t.Errorf("plan = %v, want an overlap left as it is", plan)
	}
}

// The ends of the day are not a gap: there is nothing on the other side of them
// to close against.
func TestSanitizeLeavesTheEndsOfTheDayAlone(t *testing.T) {
	loc := berlin(t)
	s := tidy(loc)

	plan := planFor(t, s,
		tracked("Standup", at(loc, 9, 0, 0), at(loc, 10, 0, 0)),
		tracked("Common core mappings", at(loc, 10, 0, 0), at(loc, 12, 0, 0)),
		tracked("Note", at(loc, 16, 0, 0), at(loc, 16, 5, 0)),
	)

	if got := plan["Note"]; got != "12:00-16:05" {
		t.Errorf("the last entry runs %q, want its own end left where it was", got)
	}
}

// The entries come back from the api in no documented order, and a day tidied
// out of order would hand the gaps to the wrong entries.
func TestSanitizeOrdersTheDayItself(t *testing.T) {
	loc := berlin(t)
	s := tidy(loc)

	plan := planFor(t, s,
		tracked("Common core mappings", at(loc, 11, 30, 0), at(loc, 13, 0, 0)),
		tracked("Standup", at(loc, 9, 0, 0), at(loc, 10, 30, 0)),
	)

	if got := plan["Standup"]; got != "09:00-11:30" {
		t.Errorf("the earlier entry runs %q, want it to close the gap", got)
	}
}

// Says what it did, so a listing can explain itself.
func TestSanitizeSaysWhyItMovedAnEntry(t *testing.T) {
	loc := berlin(t)
	s := tidy(loc)

	plan := s.Plan([]toggl.TimeEntry{
		tracked("Standup", at(loc, 9, 0, 0), at(loc, 10, 31, 12)),
		tracked("Common core mappings", at(loc, 11, 30, 0), at(loc, 13, 0, 0)),
	})

	if len(plan) != 1 {
		t.Fatalf("plan = %v, want the first entry moved", plan)
	}
	if note := plan[0].Note(); note != "snapped, extended forward" {
		t.Errorf("note = %q, want both reasons said", note)
	}
}

func TestParseZone(t *testing.T) {
	for name, tc := range map[string]struct {
		value   string
		want    string
		invalid bool
	}{
		"a span":           {value: "12:00-13:00", want: "12:00-13:00"},
		"bare hours":       {value: "12-13", want: "12:00-13:00"},
		"spaced":           {value: " 12:00 - 13:30 ", want: "12:00-13:30"},
		"to midnight":      {value: "22:00-24:00", want: "22:00-24:00"},
		"not a span":       {value: "12:00", invalid: true},
		"backwards":        {value: "13:00-12:00", invalid: true},
		"empty":            {value: "", invalid: true},
		"not a time":       {value: "lunch-13:00", invalid: true},
		"past midnight":    {value: "12:00-25:00", invalid: true},
		"too many minutes": {value: "12:00-13:75", invalid: true},
	} {
		t.Run(name, func(t *testing.T) {
			zone, err := ParseZone(tc.value)

			if tc.invalid {
				if err == nil {
					t.Errorf("ParseZone(%q) = %v, want an error", tc.value, zone)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseZone(%q): %v", tc.value, err)
			}
			if zone.String() != tc.want {
				t.Errorf("ParseZone(%q) = %q, want %q", tc.value, zone, tc.want)
			}
		})
	}
}

// A setting left out takes the default; one written as zero means none, and
// must not be read as unset.
func TestNewSanitizerReadsItsSettings(t *testing.T) {
	cfg := Config{Sanitize: SanitizeConfig{Short: "20m", NoWork: []string{"12:00-13:00"}}}

	s, err := NewSanitizer(&cfg)
	if err != nil {
		t.Fatalf("NewSanitizer: %v", err)
	}

	if s.Snap != DefaultSnap {
		t.Errorf("snap = %s, want the default %s", s.Snap, DefaultSnap)
	}
	if s.Short != 20*time.Minute {
		t.Errorf("short = %s, want the configured 20m", s.Short)
	}
	if len(s.Zones) != 1 || s.Zones[0].String() != "12:00-13:00" {
		t.Errorf("zones = %v, want lunch", s.Zones)
	}

	off, err := NewSanitizer(&Config{Sanitize: SanitizeConfig{Snap: "0"}})
	if err != nil {
		t.Fatalf("NewSanitizer: %v", err)
	}
	if off.Snap != 0 {
		t.Errorf("snap = %s, want none", off.Snap)
	}
}

func TestNewSanitizerReportsSettingsItCannotRead(t *testing.T) {
	for name, cfg := range map[string]Config{
		"a snap that is not a duration": {Sanitize: SanitizeConfig{Snap: "soon"}},
		"a negative short":              {Sanitize: SanitizeConfig{Short: "-15m"}},
		"a zone that is not a span":     {Sanitize: SanitizeConfig{NoWork: []string{"lunch"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewSanitizer(&cfg); err == nil {
				t.Error("read it without complaint, want an error")
			}
		})
	}
}
