package cmd

import (
	"strings"
	"testing"
	"time"
)

// The reference clock for every test below: Thursday 6 August 2026, 14:16 in
// Europe/Berlin, which is CEST (UTC+2). A fixed clock keeps these from
// depending on the day - or the daylight saving period - they run in.
var referenceNow = time.Date(2026, 8, 6, 14, 16, 0, 0, berlin())

func berlin() *time.Location {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		panic(err)
	}
	return loc
}

func testConfig() *ParseArgsConfig {
	return &ParseArgsConfig{
		defaultDateFormat:     "2.1.2006",
		defaultDateTimeFormat: "2.1.2006 15:04",
		defaultLocation:       berlin(),
		now:                   func() time.Time { return referenceNow },
	}
}

// utc is the expected start time, written in UTC because ParseArgs returns UTC.
func utc(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
}

func TestParseArgs(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		start    time.Time
		duration time.Duration
		tail     string
	}{
		{
			name:     "weekday name and time range",
			args:     "yesterday 15:30-17:30",
			start:    utc(2026, time.August, 5, 13, 30), // 15:30 CEST
			duration: 2 * time.Hour,
		},
		{
			// Same arguments, reversed. This is the bug that made the result
			// depend on the order: the date branch used to reinterpret an
			// already-UTC clock reading as local time.
			name:     "time range before date",
			args:     "15:30-17:30 yesterday",
			start:    utc(2026, time.August, 5, 13, 30),
			duration: 2 * time.Hour,
		},
		{
			name:     "key between date and time",
			args:     "15:30-17:30 A-KEY yesterday",
			start:    utc(2026, time.August, 5, 13, 30),
			duration: 2 * time.Hour,
			tail:     "A-KEY",
		},
		{
			name:     "weekday resolves to the most recent one",
			args:     "wed 15:30-17:30",
			start:    utc(2026, time.August, 5, 13, 30), // Wed 5 Aug
			duration: 2 * time.Hour,
		},
		{
			name:     "weekday matching today is today",
			args:     "thu 09:00-10:00",
			start:    utc(2026, time.August, 6, 7, 0),
			duration: time.Hour,
		},
		{
			name:     "full weekday name",
			args:     "monday 09:00-10:00",
			start:    utc(2026, time.August, 3, 7, 0),
			duration: time.Hour,
		},
		{
			name:     "today",
			args:     "today 09:00 1h",
			start:    utc(2026, time.August, 6, 7, 0),
			duration: time.Hour,
		},
		{
			name:     "explicit date, time range and key",
			args:     "4.1.2021 11:30-12:30 A-KEY",
			start:    utc(2021, time.January, 4, 10, 30), // 11:30 CET (UTC+1)
			duration: time.Hour,
			tail:     "A-KEY",
		},
		{
			name:  "explicit date and start time only",
			args:  "4.1.2021 A-KEY 8:00",
			start: utc(2021, time.January, 4, 7, 0),
			tail:  "A-KEY",
		},
		{
			name:     "date, start time and duration",
			args:     "4.1.2021 15:30 2h",
			start:    utc(2021, time.January, 4, 14, 30),
			duration: 2 * time.Hour,
		},
		{
			name:     "time range with today's date implied",
			args:     "A-KEY 13:30-17:00",
			start:    utc(2026, time.August, 6, 11, 30),
			duration: 210 * time.Minute,
			tail:     "A-KEY",
		},
		{
			name:     "multi word summary",
			args:     "4.1.2021 15:30 2h This is a summary",
			start:    utc(2021, time.January, 4, 14, 30),
			duration: 2 * time.Hour,
			tail:     "This is a summary",
		},
		{
			name:     "partial date with day and month",
			args:     "5.8 09:00 1h",
			start:    utc(2026, time.August, 5, 7, 0),
			duration: time.Hour,
		},
		{
			name:     "partial date with day only",
			args:     "4 09:00 1h",
			start:    utc(2026, time.August, 4, 7, 0),
			duration: time.Hour,
		},
		{
			name:     "duration in minutes",
			args:     "09:00 90m",
			start:    utc(2026, time.August, 6, 7, 0),
			duration: 90 * time.Minute,
		},
		{
			name:     "no arguments starts now",
			args:     "",
			start:    utc(2026, time.August, 6, 12, 16), // 14:16 CEST
			duration: 0,
		},
		{
			name:  "description only keeps the current time",
			args:  "fixing the build",
			start: utc(2026, time.August, 6, 12, 16),
			tail:  "fixing the build",
		},
		{
			// Winter, so the offset is CET (+1) rather than CEST (+2). A test
			// that hardcodes one offset only passes half the year.
			name:     "date in winter uses the winter offset",
			args:     "15.1.2026 09:00 1h",
			start:    utc(2026, time.January, 15, 8, 0),
			duration: time.Hour,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var args []string
			if tc.args != "" {
				args = strings.Split(tc.args, " ")
			}

			start, duration, tail, err := ParseArgs(testConfig(), args)
			if err != nil {
				t.Fatalf("ParseArgs(%q): %v", tc.args, err)
			}

			if !start.Equal(tc.start) {
				t.Errorf("start = %s, want %s", start.Format(time.RFC3339), tc.start.Format(time.RFC3339))
			}
			if duration != tc.duration {
				t.Errorf("duration = %s, want %s", duration, tc.duration)
			}
			if got := strings.Join(tail, " "); got != tc.tail {
				t.Errorf("tail = %q, want %q", got, tc.tail)
			}
		})
	}
}

// Argument order must not change the result for any permutation.
func TestParseArgsIsOrderIndependent(t *testing.T) {
	orderings := [][]string{
		{"4.1.2021", "11:30-12:30", "A-KEY"},
		{"4.1.2021", "A-KEY", "11:30-12:30"},
		{"11:30-12:30", "4.1.2021", "A-KEY"},
		{"11:30-12:30", "A-KEY", "4.1.2021"},
		{"A-KEY", "4.1.2021", "11:30-12:30"},
		{"A-KEY", "11:30-12:30", "4.1.2021"},
	}

	want := utc(2021, time.January, 4, 10, 30)

	for _, args := range orderings {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			start, duration, tail, err := ParseArgs(testConfig(), args)
			if err != nil {
				t.Fatal(err)
			}
			if !start.Equal(want) {
				t.Errorf("start = %s, want %s", start.Format(time.RFC3339), want.Format(time.RFC3339))
			}
			if duration != time.Hour {
				t.Errorf("duration = %s, want 1h", duration)
			}
			if strings.Join(tail, " ") != "A-KEY" {
				t.Errorf("tail = %v, want [A-KEY]", tail)
			}
		})
	}
}

// Something shaped like a time but invalid is an error, not description text.
func TestParseArgsRejectsMalformedTimes(t *testing.T) {
	cases := map[string]string{
		"hour out of range":     "25:00",
		"minute out of range":   "10:75",
		"range ending too soon": "17:30-15:30",
		"range of zero length":  "10:00-10:00",
		"range with bad hour":   "10:00-26:00",
	}

	for name, arg := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := ParseArgs(testConfig(), []string{arg}); err == nil {
				t.Errorf("ParseArgs(%q) returned no error; it would have become description text", arg)
			}
		})
	}
}

func TestParseArgsKeepsUnrecognisedTextAsDescription(t *testing.T) {
	args := []string{"MOET-297", "refactored", "the", "parser"}

	_, _, tail, err := ParseArgs(testConfig(), args)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(tail, " "); got != "MOET-297 refactored the parser" {
		t.Errorf("tail = %q, want the whole phrase", got)
	}
}

func TestTryDate(t *testing.T) {
	cases := []struct {
		value string
		want  time.Time
	}{
		{"6.8.2026", time.Date(2026, time.August, 6, 0, 0, 0, 0, berlin())},
		{"6.8", time.Date(2026, time.August, 6, 0, 0, 0, 0, berlin())},
		{"4", time.Date(2026, time.August, 4, 0, 0, 0, 0, berlin())},
		{"today", time.Date(2026, time.August, 6, 0, 0, 0, 0, berlin())},
		{"yesterday", time.Date(2026, time.August, 5, 0, 0, 0, 0, berlin())},
		{"sun", time.Date(2026, time.August, 2, 0, 0, 0, 0, berlin())},
	}

	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			got, matched, err := tryDate(tc.value, testConfig())
			if err != nil {
				t.Fatal(err)
			}
			if !matched {
				t.Fatalf("tryDate(%q) did not match", tc.value)
			}
			if !got.Equal(tc.want) {
				t.Errorf("got %s, want %s", got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
		})
	}
}

func TestTryDateRejectsNonDates(t *testing.T) {
	for _, value := range []string{"A-KEY", "summary", "30422198", "2h"} {
		t.Run(value, func(t *testing.T) {
			if _, matched, _ := tryDate(value, testConfig()); matched {
				t.Errorf("tryDate(%q) matched; it should be left as description text", value)
			}
		})
	}
}

// The partial layouts follow whatever date_layout the config sets, rather than
// assuming a day-first dotted format.
func TestDatePrefixLayouts(t *testing.T) {
	cases := map[string][]string{
		"2.1.2006":   {"2.1.2006", "2.1", "2"},
		"1/2/2006":   {"1/2/2006", "1/2", "1"},
		"2006-01-02": {"2006-01-02", "2006-01", "2006"},
	}

	for layout, want := range cases {
		t.Run(layout, func(t *testing.T) {
			prefixes := datePrefixLayouts(layout)
			if len(prefixes) != len(want) {
				t.Fatalf("got %d prefixes, want %d", len(prefixes), len(want))
			}
			for i, prefix := range prefixes {
				if prefix.layout != want[i] {
					t.Errorf("prefix %d = %q, want %q", i, prefix.layout, want[i])
				}
			}
		})
	}
}

// A US-style layout must read "1/4" as January 4th, not April 1st.
func TestParseArgsHonoursConfiguredDateLayout(t *testing.T) {
	config := testConfig()
	config.defaultDateFormat = "1/2/2006"
	config.defaultDateTimeFormat = "1/2/2006 15:04"

	start, _, _, err := ParseArgs(config, []string{"1/4/2021", "09:00"})
	if err != nil {
		t.Fatal(err)
	}

	want := utc(2021, time.January, 4, 8, 0)
	if !start.Equal(want) {
		t.Errorf("start = %s, want %s", start.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}
