package cmd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fefeme/workingon/workingon"
)

var (
	isTime      = regexp.MustCompile(`^(\d{1,2}):(\d{2})$`)
	isTimeRange = regexp.MustCompile(`^(\d{1,2}:\d{2})-(\d{1,2}:\d{2})$`)
	isSeparator = regexp.MustCompile(`[^0-9A-Za-z]+`)
)

// weekdays maps both "mon" and "monday" onto time.Monday, for every day.
var weekdays = func() map[string]time.Weekday {
	names := make(map[string]time.Weekday, 14)
	for day := time.Sunday; day <= time.Saturday; day++ {
		name := strings.ToLower(day.String())
		names[name] = day
		names[name[:3]] = day
	}
	return names
}()

type ParseArgsConfig struct {
	defaultDateFormat     string
	defaultDateTimeFormat string
	defaultLocation       *time.Location

	// now overrides the clock, so tests do not depend on the day they run.
	now func() time.Time
}

func newParseArgsConfig(cfg *workingon.Config) *ParseArgsConfig {
	return &ParseArgsConfig{
		defaultDateFormat:     cfg.Settings.DateLayout,
		defaultDateTimeFormat: cfg.Settings.DateTimeLayout,
		defaultLocation:       &cfg.Settings.Location,
	}
}

func (c *ParseArgsConfig) location() *time.Location {
	if c.defaultLocation != nil {
		return c.defaultLocation
	}
	return time.Local
}

func (c *ParseArgsConfig) currentTime() time.Time {
	if c.now != nil {
		return c.now().In(c.location())
	}
	return time.Now().In(c.location())
}

// clockTime is a wall clock reading with no date or zone attached. Keeping the
// time of day separate from the date is what makes argument order irrelevant:
// the two are only combined, in one location, once every argument is read.
type clockTime struct {
	hour   int
	minute int
}

func (c clockTime) sub(other clockTime) time.Duration {
	minutes := (c.hour-other.hour)*60 + (c.minute - other.minute)
	return time.Duration(minutes) * time.Minute
}

// ParseArgs reads a free-form argument list into a start time, a duration and
// whatever text was left over.
//
// Arguments may appear in any order: "yesterday 15:30-17:30" and
// "15:30-17:30 yesterday" produce the same instant.
func ParseArgs(config *ParseArgsConfig, args []string) (time.Time, time.Duration, []string, error) {
	loc := config.location()
	now := config.currentTime()

	year, month, day := now.Date()
	clock := clockTime{hour: now.Hour(), minute: now.Minute()}

	var (
		duration time.Duration
		tail     []string
	)

	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}

		if from, span, matched, err := tryTimeRange(arg); matched {
			if err != nil {
				return time.Time{}, 0, nil, err
			}
			clock, duration = from, span
			continue
		}

		if at, matched, err := tryClock(arg); matched {
			if err != nil {
				return time.Time{}, 0, nil, err
			}
			clock = at
			continue
		}

		if span, err := time.ParseDuration(arg); err == nil && span > 0 {
			duration = span
			continue
		}

		if date, matched, err := tryDate(arg, config); matched {
			if err != nil {
				return time.Time{}, 0, nil, err
			}
			year, month, day = date.Date()
			continue
		}

		tail = append(tail, arg)
	}

	start := time.Date(year, month, day, clock.hour, clock.minute, 0, 0, loc)

	return start.UTC(), duration, tail, nil
}

// tryClock reads "15:30". The bool reports whether the argument looked like a
// time at all, so a malformed one is rejected rather than silently treated as
// part of the description.
func tryClock(value string) (clockTime, bool, error) {
	match := isTime.FindStringSubmatch(value)
	if match == nil {
		return clockTime{}, false, nil
	}

	hour, _ := strconv.Atoi(match[1])
	minute, _ := strconv.Atoi(match[2])

	if hour > 23 || minute > 59 {
		return clockTime{}, true, fmt.Errorf("%q is not a valid time of day", value)
	}

	return clockTime{hour: hour, minute: minute}, true, nil
}

// tryTimeRange reads "15:30-17:30" into a start time and a duration.
func tryTimeRange(value string) (clockTime, time.Duration, bool, error) {
	match := isTimeRange.FindStringSubmatch(value)
	if match == nil {
		return clockTime{}, 0, false, nil
	}

	from, _, err := tryClock(match[1])
	if err != nil {
		return clockTime{}, 0, true, err
	}

	to, _, err := tryClock(match[2])
	if err != nil {
		return clockTime{}, 0, true, err
	}

	duration := to.sub(from)
	if duration <= 0 {
		return clockTime{}, 0, true, fmt.Errorf("%q ends at or before it starts", value)
	}

	return from, duration, true, nil
}

type datePrefix struct {
	layout   string
	hasMonth bool
	hasYear  bool
}

// datePrefixLayouts derives the configured date layout and its shorter
// prefixes, longest first, so a layout of "2.1.2006" also accepts "6.8" and
// "6". Anything the prefix omits is filled in from the current date.
func datePrefixLayouts(layout string) []datePrefix {
	fields := isSeparator.Split(layout, -1)
	separators := isSeparator.FindAllString(layout, -1)

	var prefixes []datePrefix

	for count := len(fields); count >= 1; count-- {
		var (
			builder strings.Builder
			prefix  datePrefix
			known   = true
		)

		for i := 0; i < count; i++ {
			if i > 0 {
				builder.WriteString(separators[i-1])
			}
			builder.WriteString(fields[i])

			switch dateFieldKind(fields[i]) {
			case "month":
				prefix.hasMonth = true
			case "year":
				prefix.hasYear = true
			case "day":
			default:
				known = false
			}
		}

		if !known {
			continue
		}

		prefix.layout = builder.String()
		prefixes = append(prefixes, prefix)
	}

	return prefixes
}

func dateFieldKind(field string) string {
	switch field {
	case "2", "02", "_2":
		return "day"
	case "1", "01", "Jan", "January":
		return "month"
	case "2006", "06":
		return "year"
	}
	return ""
}

// tryDate reads a date argument: a full or partial date in the configured
// layout, a weekday name, "today" or "yesterday".
func tryDate(value string, config *ParseArgsConfig) (time.Time, bool, error) {
	loc := config.location()
	now := config.currentTime()

	switch strings.ToLower(value) {
	case "today":
		return startOfDay(now, loc), true, nil
	case "yesterday":
		return startOfDay(now.AddDate(0, 0, -1), loc), true, nil
	}

	if weekday, exists := weekdays[strings.ToLower(value)]; exists {
		// The most recent such weekday, which is today if today matches.
		day := now
		for day.Weekday() != weekday {
			day = day.AddDate(0, 0, -1)
		}
		return startOfDay(day, loc), true, nil
	}

	for _, prefix := range datePrefixLayouts(config.defaultDateFormat) {
		parsed, err := time.ParseInLocation(prefix.layout, value, loc)
		if err != nil {
			continue
		}

		year, month, day := parsed.Date()
		if !prefix.hasYear {
			year = now.Year()
		}
		if !prefix.hasMonth {
			month = now.Month()
		}

		return time.Date(year, month, day, 0, 0, 0, 0, loc), true, nil
	}

	return time.Time{}, false, nil
}

func ParseDateFromArg(date string, cfg *workingon.Config) (time.Time, error) {
	parsed, matched, err := tryDate(date, newParseArgsConfig(cfg))
	if err != nil {
		return time.Time{}, err
	}
	if !matched {
		return time.Time{}, fmt.Errorf("unable to read %q as a date", date)
	}
	return parsed, nil
}

func startOfDay(t time.Time, loc *time.Location) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, loc)
}
