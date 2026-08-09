package workingon

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fefeme/workingon/toggl"
)

// How a day is tidied when nothing says otherwise: times land on a five minute
// grid, and anything under a quarter of an hour is a stub rather than a piece
// of work in its own right.
const (
	DefaultSnap  = 5 * time.Minute
	DefaultShort = 15 * time.Minute
)

// Zone is a span of the day nothing may be stretched into - lunch, most often.
//
// It is a time of day rather than an instant, so one zone stands for the same
// hours of whichever day is being tidied.
type Zone struct {
	// FromMinute and ToMinute are minutes since midnight, so a zone survives
	// the clocks going forward: it is 12:00 that lunch starts at, not a fixed
	// number of hours after the day began.
	FromMinute int
	ToMinute   int
}

func (z Zone) String() string {
	return fmt.Sprintf("%02d:%02d-%02d:%02d",
		z.FromMinute/60, z.FromMinute%60, z.ToMinute/60, z.ToMinute%60)
}

func (z Zone) on(day time.Time) (time.Time, time.Time) {
	year, month, date := day.Date()
	loc := day.Location()

	return time.Date(year, month, date, 0, z.FromMinute, 0, 0, loc),
		time.Date(year, month, date, 0, z.ToMinute, 0, 0, loc)
}

// ParseZone reads "12:00-13:00". A bare hour stands for the whole hour, and an
// end of 24:00 is midnight.
func ParseZone(value string) (Zone, error) {
	from, to, ranged := strings.Cut(value, "-")
	if !ranged {
		return Zone{}, fmt.Errorf("no_work %q: a zone is a span, as in \"12:00-13:00\"", value)
	}

	fromMinute, err := parseClockMinute(from)
	if err != nil {
		return Zone{}, fmt.Errorf("no_work %q: %w", value, err)
	}

	toMinute, err := parseClockMinute(to)
	if err != nil {
		return Zone{}, fmt.Errorf("no_work %q: %w", value, err)
	}

	if toMinute <= fromMinute {
		return Zone{}, fmt.Errorf("no_work %q: it ends at or before it starts", value)
	}

	return Zone{FromMinute: fromMinute, ToMinute: toMinute}, nil
}

func ParseZones(values []string) ([]Zone, error) {
	var zones []Zone

	for _, value := range values {
		zone, err := ParseZone(strings.TrimSpace(value))
		if err != nil {
			return nil, err
		}
		zones = append(zones, zone)
	}

	return zones, nil
}

// parseClockMinute reads "12:30", or "12" for the top of the hour, as minutes
// since midnight.
func parseClockMinute(value string) (int, error) {
	value = strings.TrimSpace(value)

	hours, minutes, hasMinutes := strings.Cut(value, ":")
	if !hasMinutes {
		minutes = "0"
	}

	hour, hourErr := strconv.Atoi(strings.TrimSpace(hours))
	minute, minuteErr := strconv.Atoi(strings.TrimSpace(minutes))

	if hourErr != nil || minuteErr != nil || hour < 0 || minute < 0 || minute > 59 ||
		hour*60+minute > 24*60 {
		return 0, fmt.Errorf("%q is not a time of day", value)
	}

	return hour*60 + minute, nil
}

// Sanitizer tidies a day's worth of time entries: it rounds ragged times off
// and hands the gaps between entries to whichever entry they belong to.
//
// It only ever moves the ends of entries that are already there. Nothing is
// created, nothing is deleted, and an entry that overlaps a no work zone
// because that is when you worked is left exactly as it is.
type Sanitizer struct {
	// Snap is the grid start and stop times are rounded to. Zero leaves them
	// where they are.
	Snap time.Duration

	// Short is how long an entry has to run to count as work in its own right.
	// Anything under it is a stub - a note typed while doing something else -
	// and the gaps around it are taken to be its own.
	Short time.Duration

	Zones []Zone

	// DayEnds is the time of day work stops, in minutes since midnight. An
	// entry that outlives it is cut back to it, and a timer still running past
	// it is ended there - which is what a day left running overnight looks
	// like. Nil where no such time is set, and nothing is capped.
	//
	// It is a time of day rather than an instant, for the same reason a Zone
	// is: it is 18:00 that the day ends at, whichever day is being tidied.
	DayEnds *int

	// Location is the zone the day is read in.
	Location *time.Location

	// Now is where a running timer has got to, for tests that cannot wait.
	Now func() time.Time
}

func NewSanitizer(cfg *Config) (Sanitizer, error) {
	snap, err := parseTidyDuration("snap", cfg.Sanitize.Snap, DefaultSnap)
	if err != nil {
		return Sanitizer{}, err
	}

	short, err := parseTidyDuration("short", cfg.Sanitize.Short, DefaultShort)
	if err != nil {
		return Sanitizer{}, err
	}

	zones, err := ParseZones(cfg.Sanitize.NoWork)
	if err != nil {
		return Sanitizer{}, err
	}

	dayEnds, err := parseDayEnds(cfg.Sanitize.DayEnds)
	if err != nil {
		return Sanitizer{}, err
	}

	return Sanitizer{
		Snap:     snap,
		Short:    short,
		Zones:    zones,
		DayEnds:  dayEnds,
		Location: &cfg.Settings.Location,
	}, nil
}

// parseDayEnds reads the time of day work stops. Left out, there is no such
// time and nothing is capped - which is why this answers with a pointer rather
// than reaching for a default. Guessing when someone's day ends and quietly
// shortening their entries by it is not a thing to do uninvited.
func parseDayEnds(value string) (*int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	minute, err := parseClockMinute(value)
	if err != nil {
		return nil, fmt.Errorf("sanitize day_ends: %w", err)
	}
	if minute == 0 {
		return nil, fmt.Errorf("sanitize day_ends: %q would end the day before it began", value)
	}

	return &minute, nil
}

// parseTidyDuration reads a setting that is a duration, taking the default only
// when it was left out. A setting written as "0" is a deliberate none, and must
// not be read as an unset one.
func parseTidyDuration(name, value string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("sanitize %s: %q is not a duration, as in \"5m\"", name, value)
	}
	if parsed < 0 {
		return 0, fmt.Errorf("sanitize %s: %q is negative", name, value)
	}

	return parsed, nil
}

// Adjustment is one entry, and where it ought to run instead.
type Adjustment struct {
	Entry toggl.TimeEntry
	Start time.Time
	Stop  time.Time
	Notes []string
}

func (a Adjustment) Note() string {
	return strings.Join(a.Notes, ", ")
}

// span is an entry placed on the day, with where tidying has moved it to so far.
type span struct {
	entry     toggl.TimeEntry
	begin     time.Time
	finish    time.Time
	newBegin  time.Time
	newFinish time.Time

	// fixed marks an entry that is not ours to move: a timer still running has
	// no end to place, and where it began is where you began it.
	fixed bool

	// ceiling is as far forward as this entry may run, for one that was cut
	// back to the end of the day. Without it the tidying that follows would
	// hand straight back the evening the cap just took away. Zero where
	// nothing bounds the entry.
	ceiling time.Time

	notes []string
}

// under holds a time to the entry's ceiling, where it has one.
func (s *span) under(moment time.Time) time.Time {
	if !s.ceiling.IsZero() && moment.After(s.ceiling) {
		return s.ceiling
	}
	return moment
}

func (s *span) note(what string) {
	for _, already := range s.notes {
		if already == what {
			return
		}
	}
	s.notes = append(s.notes, what)
}

// It is asked of the entry as tracked, so an entry does not stop being a stub
// halfway through being grown.
func (s *span) isStub(short time.Duration) bool {
	return short > 0 && s.finish.Sub(s.begin) < short
}

func (s *span) stretchForward(to time.Time) {
	to = s.under(to)
	if !to.After(s.newFinish) {
		return
	}
	s.newFinish = to
	s.note("extended forward")
}

func (s *span) stretchBack(to time.Time) {
	if !to.Before(s.newBegin) {
		return
	}
	s.newBegin = to
	s.note("extended back")
}

// Plan is what tidying a day would change, and nothing more - it reads the
// entries and answers, leaving the saving to the caller.
func (s Sanitizer) Plan(entries []toggl.TimeEntry) []Adjustment {
	spans := s.spans(entries)

	s.snapToGrid(spans)
	s.closeGaps(spans)

	var plan []Adjustment
	for _, sp := range spans {
		if sp.newBegin.Equal(sp.begin) && sp.newFinish.Equal(sp.finish) {
			continue
		}

		plan = append(plan, Adjustment{
			Entry: sp.entry,
			Start: sp.newBegin,
			Stop:  sp.newFinish,
			Notes: sp.notes,
		})
	}

	return plan
}

func (s Sanitizer) spans(entries []toggl.TimeEntry) []*span {
	loc := s.location()

	var spans []*span
	for _, entry := range entries {
		if entry.Start == nil {
			continue
		}

		begin := entry.Start.In(loc)
		finish := begin.Add(time.Duration(entry.Duration) * time.Second)

		// A running timer has got as far as it has got. It is left where it is,
		// but it still walls off the time after it, so nothing is stretched
		// over the entry you are in the middle of.
		if entry.IsRunning() {
			finish = s.now().In(loc)
			if finish.Before(begin) {
				finish = begin
			}
		}

		sp := &span{
			entry: entry, begin: begin, finish: finish,
			newBegin: begin, newFinish: finish,
			fixed: entry.IsRunning(),
		}
		s.capToDayEnd(sp)

		spans = append(spans, sp)
	}

	sort.SliceStable(spans, func(i, j int) bool {
		return spans[i].begin.Before(spans[j].begin)
	})

	return spans
}

// This is what a timer left running overnight comes to. It did not run until
// you opened the laptop again the next morning; it ran until you left, and the
// end of the day is the closest thing to that anyone can say afterwards. A
// timer still going is ended there too - the entry has already outlived the
// day it belongs to, so there is nothing left worth leaving alone.
func (s Sanitizer) capToDayEnd(sp *span) {
	if s.DayEnds == nil {
		return
	}

	end := startOfDate(sp.begin).Add(time.Duration(*s.DayEnds) * time.Minute)

	// An entry that began after the day was already over is not one this can
	// help: there is no honest end to give it, and pulling it back to before it
	// started would be worse than the overlong entry it replaced.
	if !sp.finish.After(end) || !end.After(sp.begin) {
		return
	}

	if sp.fixed {
		sp.note("stopped at end of day")
	} else {
		sp.note("capped at end of day")
	}

	// Cut short, it is an ordinary entry again - it has an end, so the rest of
	// the tidying may round it off and close the gaps around it as it would any
	// other. The ceiling is what keeps that from undoing the cap.
	sp.finish, sp.newFinish = end, end
	sp.fixed = false
	sp.ceiling = end
}

// snapToGrid rounds ragged times off, so a day reads as clock times rather than
// as stopwatch readings.
func (s Sanitizer) snapToGrid(spans []*span) {
	if s.Snap <= 0 {
		return
	}

	for i, sp := range spans {
		if sp.fixed {
			continue
		}

		begin := roundTo(sp.begin, s.Snap)

		// Rounding an end of day that does not sit on the grid would carry the
		// entry back past the cap it was just given.
		finish := sp.under(roundTo(sp.finish, s.Snap))

		// An entry shorter than the grid would round away to nothing. It is a
		// stub, and closing the gaps around it is about to give it a length -
		// rounding it out of existence first would only lose it.
		if !finish.After(begin) {
			continue
		}

		// Rounding keeps two times in the order it found them, so this can only
		// reach back over the entry before it where that one kept its own
		// times. Leave it be: an overlap that was already there is not what
		// tidying is for.
		if i > 0 && begin.Before(spans[i-1].newFinish) {
			continue
		}

		sp.newBegin, sp.newFinish = begin, finish
		sp.note("snapped")
	}
}

// A gap before a stub is the stub's: an entry of a few minutes is a note about
// something that took longer than that, so it grows to meet the entries either
// side of it. Every other gap goes to the entry that ran into it, which is the
// usual case of having carried on working without saying so.
func (s Sanitizer) closeGaps(spans []*span) {
	for i := 0; i+1 < len(spans); i++ {
		left, right := spans[i], spans[i+1]

		gapFrom, gapTo := left.newFinish, right.newBegin
		if !gapTo.After(gapFrom) {
			continue
		}

		free := s.freeRuns(gapFrom, gapTo)
		if len(free) == 0 {
			continue
		}

		head, tail := free[0], free[len(free)-1]
		fromHead := head.from.Equal(gapFrom)
		toTail := tail.to.Equal(gapTo)

		// One run either side could reach is the ordinary gap, and only one of
		// them can have it.
		if len(free) == 1 && fromHead && toTail {
			switch {
			case right.isStub(s.Short) && !right.fixed:
				right.stretchBack(gapFrom)
			case !left.fixed:
				left.stretchForward(gapTo)
			case !right.fixed:
				right.stretchBack(gapFrom)
			}
			continue
		}

		// A no work zone breaks the gap up, and each side closes what it can
		// reach without crossing one. What neither can reach stays a gap, which
		// is what the zone is there for.
		if fromHead && !left.fixed {
			left.stretchForward(head.to)
		}
		if toTail && !right.fixed {
			right.stretchBack(tail.from)
		}
	}
}

// run is a stretch of time with nothing in it.
type run struct {
	from time.Time
	to   time.Time
}

// freeRuns is what a gap comes to once the no work zones are taken out of it.
func (s Sanitizer) freeRuns(from, to time.Time) []run {
	runs := []run{{from: from, to: to}}

	for _, zone := range s.zonesBetween(from, to) {
		var left []run
		for _, free := range runs {
			left = append(left, free.without(zone.from, zone.to)...)
		}
		runs = left
	}

	return runs
}

// zonesBetween places every zone on the dates a gap covers - an entry can run
// past midnight, and the zones of the day it ends on apply just as much.
func (s Sanitizer) zonesBetween(from, to time.Time) []run {
	var placed []run

	for date := startOfDate(from); !date.After(to); date = date.AddDate(0, 0, 1) {
		for _, zone := range s.Zones {
			zoneFrom, zoneTo := zone.on(date)
			placed = append(placed, run{from: zoneFrom, to: zoneTo})
		}
	}

	return placed
}

// without is what is left of a run once another is taken out of it.
func (r run) without(from, to time.Time) []run {
	if !to.After(r.from) || !r.to.After(from) {
		return []run{r}
	}

	var left []run
	if r.from.Before(from) {
		left = append(left, run{from: r.from, to: from})
	}
	if to.Before(r.to) {
		left = append(left, run{from: to, to: r.to})
	}

	return left
}

// The grid starts at midnight rather than at the epoch so that it lines up with
// the clock in a zone whose offset is not a whole number of hours, and the
// result is built as a date so that the day the clocks change is still a day.
func roundTo(moment time.Time, grid time.Duration) time.Time {
	if grid <= 0 {
		return moment
	}

	midnight := startOfDate(moment)
	offset := moment.Sub(midnight)
	rounded := ((offset + grid/2) / grid) * grid

	year, month, date := midnight.Date()

	return time.Date(year, month, date, 0, 0, int(rounded/time.Second), 0, moment.Location())
}

func startOfDate(moment time.Time) time.Time {
	year, month, date := moment.Date()
	return time.Date(year, month, date, 0, 0, 0, 0, moment.Location())
}

func (s Sanitizer) location() *time.Location {
	if s.Location == nil {
		return time.Local
	}
	return s.Location
}

func (s Sanitizer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func Sanitize(cfg *Config, adjustment Adjustment) (*toggl.TimeEntry, error) {
	return sanitize(toggl.NewToggl(cfg.Settings.ToggleApiToken), adjustment)
}

func sanitize(client *toggl.Toggl, adjustment Adjustment) (*toggl.TimeEntry, error) {
	entry := adjustment.Entry

	start := adjustment.Start.UTC()
	stop := adjustment.Stop.UTC()

	entry.Start = &start
	entry.Stop = &stop
	entry.Duration = int64(stop.Sub(start).Seconds())

	return client.TimeEntries.Update(&entry)
}
