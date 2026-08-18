package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/fefeme/workingon/workingon"
)

// The one field a caller checks before deciding whether this checkout is one
// that books time. It has to agree with the overlay actually having been found.
func TestWhereSaysWhetherTheCheckoutIsConfigured(t *testing.T) {
	for name, tc := range map[string]struct {
		local string
		want  bool
	}{
		"an overlay was found": {local: "/src/project/.workingon.yaml", want: true},
		"none was":             {want: false},
	} {
		t.Run(name, func(t *testing.T) {
			where := whereJSON{LocalConfig: tc.local, Configured: tc.local != ""}

			if where.Configured != tc.want {
				t.Errorf("configured = %v, want %v", where.Configured, tc.want)
			}
		})
	}
}

// A checkout with no overlay is not a failure to report, it is an answer - and
// one that should say what to do about it.
func TestWhereSaysWhatToDoWithAnUnconfiguredCheckout(t *testing.T) {
	out := renderWhere(whereJSON{Directory: "/src/project"})

	for _, want := range []string{"no .workingon.yaml", "wo init"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not mention %q:\n%s", want, out)
		}
	}
}

func TestWhereNamesTheProjectEntriesWouldLandIn(t *testing.T) {
	for name, tc := range map[string]struct {
		where whereJSON
		want  string
	}{
		"a project that resolved": {
			where: whereJSON{Project: &ref{Id: 42, Name: "Learning Platform"}},
			want:  "Learning Platform (42)",
		},
		"an id that did not": {
			where: whereJSON{Project: &ref{Id: 42}},
			want:  "42",
		},
		"no project at all": {
			where: whereJSON{},
			want:  "no toggl_default_pid is set",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if out := renderWhere(tc.where); !strings.Contains(out, tc.want) {
				t.Errorf("output does not mention %q:\n%s", tc.want, out)
			}
		})
	}
}

// The repository is where the overlay is, not where you happened to run from -
// the walk goes up, so the two are usually different.
func TestWhereNamesTheRepositoryRatherThanTheDirectory(t *testing.T) {
	out := renderWhere(whereJSON{
		Directory:   "/src/project/deep/nested",
		LocalConfig: "/src/project/.workingon.yaml",
		Configured:  true,
	})

	if !strings.Contains(out, "/src/project\n") {
		t.Errorf("output does not name the repository root:\n%s", out)
	}
	if !strings.Contains(out, "/src/project/deep/nested") {
		t.Errorf("output does not name the directory it was run from:\n%s", out)
	}
}

// The one thing this must never do. `wo where --show` is what someone pastes
// into a bug report.
func TestShowNeverPrintsTheApiToken(t *testing.T) {
	cfg := &workingon.Config{}
	cfg.Settings.ToggleApiToken = "1234567890abcdef"

	view, err := settingsInForce(cfg)
	if err != nil {
		t.Fatalf("settingsInForce: %v", err)
	}
	if !view.Settings.TokenSet {
		t.Error("token_set = false, want it reported as present")
	}

	out := renderWhere(whereJSON{Config: view})
	if strings.Contains(out, "1234567890abcdef") {
		t.Errorf("the token is in the output:\n%s", out)
	}
	if !strings.Contains(out, "toggl_api_token            set") {
		t.Errorf("output does not say a token is set:\n%s", out)
	}

	cfg.Settings.ToggleApiToken = "   "
	blank, err := settingsInForce(cfg)
	if err != nil {
		t.Fatalf("settingsInForce: %v", err)
	}
	if blank.Settings.TokenSet {
		t.Error("token_set = true for whitespace, want false")
	}
}

// What is shown has to be what tidying would run with, defaults and all -
// showing the empty strings from the file would answer a different question.
func TestShowResolvesSanitizeDefaults(t *testing.T) {
	view, err := settingsInForce(&workingon.Config{})
	if err != nil {
		t.Fatalf("settingsInForce: %v", err)
	}

	if view.Sanitize.Snap != "5m" {
		t.Errorf("snap = %q, want the default 5m", view.Sanitize.Snap)
	}
	if view.Sanitize.Short != "15m" {
		t.Errorf("short = %q, want the default 15m", view.Sanitize.Short)
	}
	if view.Sanitize.DayEnds != "" {
		t.Errorf("day_ends = %q, want none where none is set", view.Sanitize.DayEnds)
	}
	if view.Sanitize.NoWork == nil {
		t.Error("no_work = null, want an empty list")
	}
}

func TestShowReadsTheSanitizeSettings(t *testing.T) {
	cfg := &workingon.Config{}
	cfg.Sanitize = workingon.SanitizeConfig{
		Snap:    "0",
		Short:   "1h30m",
		NoWork:  []string{"12:00-13:00"},
		DayEnds: "17:30",
	}

	view, err := settingsInForce(cfg)
	if err != nil {
		t.Fatalf("settingsInForce: %v", err)
	}

	for what, got := range map[string]string{
		"snap":     view.Sanitize.Snap,
		"short":    view.Sanitize.Short,
		"day_ends": view.Sanitize.DayEnds,
		"no_work":  strings.Join(view.Sanitize.NoWork, ","),
	} {
		want := map[string]string{
			"snap": "0", "short": "1h30m", "day_ends": "17:30", "no_work": "12:00-13:00",
		}[what]

		if got != want {
			t.Errorf("%s = %q, want %q", what, got, want)
		}
	}
}

// A setting that cannot be read is a fault to report, not something to show a
// half-parsed version of - `wo sanitize` would fail on the same value.
func TestShowFailsOnAConfigItCannotRead(t *testing.T) {
	cfg := &workingon.Config{}
	cfg.Sanitize = workingon.SanitizeConfig{Snap: "half an hour"}

	if _, err := settingsInForce(cfg); err == nil {
		t.Fatal("settingsInForce accepted a snap that is not a duration")
	}
}

// The ordinary answer is about this directory. Settings only appear when asked
// for, so a caller parsing `wo where --json` sees no difference.
func TestWhereLeavesTheSettingsOutUnlessAsked(t *testing.T) {
	out := renderWhere(whereJSON{Directory: "/src/project"})

	if strings.Contains(out, "Sanitize") {
		t.Errorf("settings appear without --show:\n%s", out)
	}
}

func TestSettingDurationWritesWhatTheConfigWouldSay(t *testing.T) {
	for _, tc := range []struct {
		given time.Duration
		want  string
	}{
		{0, "0"},
		{-time.Minute, "0"},
		{5 * time.Minute, "5m"},
		{30 * time.Second, "30s"},
		{90 * time.Minute, "1h30m"},
		{time.Hour, "1h"},
		{time.Hour + 30*time.Second, "1h30s"},
	} {
		if got := settingDuration(tc.given); got != tc.want {
			t.Errorf("settingDuration(%s) = %q, want %q", tc.given, got, tc.want)
		}
	}
}

// A value nobody set reads as such, rather than as blank space that looks like
// a rendering fault.
func TestShowSaysWhenAValueIsNotSet(t *testing.T) {
	view, err := settingsInForce(&workingon.Config{})
	if err != nil {
		t.Fatalf("settingsInForce: %v", err)
	}

	out := renderWhere(whereJSON{Config: view})
	for _, want := range []string{"toggl_default_task         not set", "day_ends                   not set"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not show %q:\n%s", want, out)
		}
	}
}
