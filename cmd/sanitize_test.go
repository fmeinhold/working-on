package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"
)

func sanitizeConfig() *workingon.Config {
	cfg := &workingon.Config{}
	cfg.Settings.DateLayout = "2.1.2006"
	cfg.Settings.DateTimeLayout = "2.1.2006 15:04"
	cfg.Settings.Location = *time.UTC

	return cfg
}

func sanitizeEntry(description string, hour, minute int, duration time.Duration) toggl.TimeEntry {
	start := time.Date(2026, time.August, 7, hour, minute, 0, 0, time.UTC)
	stop := start.Add(duration)

	return toggl.TimeEntry{
		Id: 1, WorkspaceId: 1562374, Description: description,
		Start: &start, Stop: &stop, Duration: int64(duration.Seconds()),
	}
}

func TestRenderSanitizePlanShowsBeforeAndAfter(t *testing.T) {
	cfg := sanitizeConfig()
	day := time.Date(2026, time.August, 7, 0, 0, 0, 0, time.UTC)

	entry := sanitizeEntry("Research into state codes", 11, 15, 5*time.Minute)
	plan := []workingon.Adjustment{{
		Entry: entry,
		Start: time.Date(2026, time.August, 7, 10, 30, 0, 0, time.UTC),
		Stop:  time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC),
		Notes: []string{"extended back", "extended forward"},
	}}

	out := RenderSanitizePlan(day, plan, workingon.Sanitizer{}, cfg, noNames)

	for _, want := range []string{
		"7.8.2026", "1 entry to tidy",
		"11:15-11:20 (5m)", "10:30-12:00 (1h 30m)",
		"Research into state codes", "extended back, extended forward",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the listing does not say %q:\n%s", want, out)
		}
	}
}

func TestRenderSanitizePlanSaysWhenThereIsNothingToDo(t *testing.T) {
	day := time.Date(2026, time.August, 7, 0, 0, 0, 0, time.UTC)

	out := RenderSanitizePlan(day, nil, workingon.Sanitizer{}, sanitizeConfig(), noNames)

	if !strings.Contains(out, "Nothing to tidy on Friday, 7.8.2026") {
		t.Errorf("out = %q, want it to say the day is already tidy", out)
	}
}

// The zones are worth saying out loud: a gap left open is otherwise
// indistinguishable from one that was missed.
func TestRenderSanitizePlanNamesTheNoWorkZones(t *testing.T) {
	cfg := sanitizeConfig()
	day := time.Date(2026, time.August, 7, 0, 0, 0, 0, time.UTC)

	plan := []workingon.Adjustment{{
		Entry: sanitizeEntry("Standup", 9, 0, time.Hour),
		Start: time.Date(2026, time.August, 7, 9, 0, 0, 0, time.UTC),
		Stop:  time.Date(2026, time.August, 7, 12, 0, 0, 0, time.UTC),
		Notes: []string{"extended forward"},
	}}
	sanitizer := workingon.Sanitizer{Zones: []workingon.Zone{{FromMinute: 12 * 60, ToMinute: 13 * 60}}}

	out := RenderSanitizePlan(day, plan, sanitizer, cfg, noNames)

	if !strings.Contains(out, "Nothing was stretched into 12:00-13:00") {
		t.Errorf("the listing does not mention the zone:\n%s", out)
	}
}

// Nothing is saved without being asked about, and a run with nobody to ask says
// so rather than guessing.
func TestConfirmSanitizeAsksBeforeSaving(t *testing.T) {
	for name, tc := range map[string]struct {
		answer      string
		interactive bool
		yes         bool
		want        bool
		says        string
	}{
		"answered yes":     {answer: "y\n", interactive: true, want: true},
		"answered no":      {answer: "n\n", interactive: true, want: false},
		"left blank":       {answer: "\n", interactive: true, want: false},
		"nobody to ask":    {answer: "", interactive: false, want: false, says: "--yes"},
		"said so up front": {answer: "", interactive: false, yes: true, want: true},
	} {
		t.Run(name, func(t *testing.T) {
			out := &bytes.Buffer{}

			got := confirmSanitizeWith(strings.NewReader(tc.answer), out,
				tc.interactive, tc.yes, 2)

			if got != tc.want {
				t.Errorf("confirmed = %v, want %v", got, tc.want)
			}
			if tc.says != "" && !strings.Contains(out.String(), tc.says) {
				t.Errorf("out = %q, want it to mention %q", out, tc.says)
			}
		})
	}
}

// A flag is for this one run, and says what the config does not.
func TestSanitizeFlagsOverrideTheConfig(t *testing.T) {
	cfg := sanitizeConfig()
	cfg.Sanitize = workingon.SanitizeConfig{Snap: "5m", Short: "15m"}

	command := NewSanitizeCommand(cfg)
	if err := command.Flags().Set("snap", "0"); err != nil {
		t.Fatal(err)
	}

	sanitizer, err := newSanitizer(cfg, command, "0", "")
	if err != nil {
		t.Fatalf("newSanitizer: %v", err)
	}

	if sanitizer.Snap != 0 {
		t.Errorf("snap = %s, want the flag's none", sanitizer.Snap)
	}
	if sanitizer.Short != 15*time.Minute {
		t.Errorf("short = %s, want the configured 15m", sanitizer.Short)
	}
}
