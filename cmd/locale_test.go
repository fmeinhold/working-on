package cmd

import (
	"testing"
	"time"
)

// isolateLocale keeps the machine running the test out of it: whatever it has
// set in the environment, and on darwin its own AppleLocale.
func isolateLocale(t *testing.T) {
	t.Helper()

	t.Setenv("LC_ALL", "")
	t.Setenv("LC_TIME", "")
	t.Setenv("LANG", "")

	previous := appleLocale
	appleLocale = func() string { return "" }
	t.Cleanup(func() { appleLocale = previous })
}

func TestLocaleDateLayout(t *testing.T) {
	isolateLocale(t)

	for _, tc := range []struct {
		locale string
		want   string
	}{
		{"de_DE.UTF-8", "2.1.2006"},
		{"en_US.UTF-8", "1/2/2006"},
		{"en_GB.UTF-8", "2/1/2006"},
		{"nl_NL", "2-1-2006"},
		{"sv_SE", "2006-01-02"},
		{"ja_JP.UTF-8", "2006/1/2"},

		// A region the table does not name falls back rather than guessing.
		{"en_ZW", fallbackDateLayout},

		// No region, so the language decides.
		{"fr", "2/1/2006"},
		{"de", "2.1.2006"},

		// Neither of these says where the machine is.
		{"C", fallbackDateLayout},
		{"", fallbackDateLayout},
	} {
		t.Setenv("LANG", tc.locale)

		if got := localeDateLayout(); got != tc.want {
			t.Errorf("localeDateLayout() for %q = %q, want %q", tc.locale, got, tc.want)
		}
	}
}

func TestLocaleTimeLayout(t *testing.T) {
	isolateLocale(t)

	for _, tc := range []struct {
		locale string
		want   string
	}{
		{"en_US.UTF-8", "3:04 PM"},
		{"en_GB.UTF-8", "15:04"},
		{"de_DE.UTF-8", "15:04"},

		// An unknown region and an unreadable locale fall back the way the
		// date does, so the pair is one convention.
		{"en_ZW", fallbackTimeLayout},
		{"", fallbackTimeLayout},
	} {
		t.Setenv("LANG", tc.locale)

		if got := localeTimeLayout(); got != tc.want {
			t.Errorf("localeTimeLayout() for %q = %q, want %q", tc.locale, got, tc.want)
		}
	}
}

// LC_ALL beats LC_TIME beats LANG, and an empty one is not an answer.
func TestCurrentLocaleOrder(t *testing.T) {
	isolateLocale(t)

	t.Setenv("LANG", "de_DE.UTF-8")
	t.Setenv("LC_TIME", "")
	t.Setenv("LC_ALL", "")

	if got := currentLocale(); got != "de_DE" {
		t.Errorf("currentLocale() = %q, want de_DE", got)
	}

	t.Setenv("LC_TIME", "en_GB.UTF-8")
	if got := currentLocale(); got != "en_GB" {
		t.Errorf("currentLocale() = %q, want en_GB", got)
	}

	t.Setenv("LC_ALL", "en_US.UTF-8")
	if got := currentLocale(); got != "en_US" {
		t.Errorf("currentLocale() = %q, want en_US", got)
	}
}

func TestCurrentLocaleFallsBackToMacOS(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_TIME", "")
	t.Setenv("LANG", "")

	previous := appleLocale
	appleLocale = func() string { return "en_US@currency=USD" }
	t.Cleanup(func() { appleLocale = previous })

	if got := currentLocale(); got != "en_US" {
		t.Errorf("currentLocale() = %q, want en_US", got)
	}
}

func TestNormalizeLocale(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"de_DE.UTF-8", "de_DE"},
		{"en-GB", "en_GB"},
		{"de_DE.UTF-8@euro", "de_DE"},
		{"  en_US  ", "en_US"},
		{"POSIX", ""},
		{"c", ""},
	} {
		if got := normalizeLocale(tc.in); got != tc.want {
			t.Errorf("normalizeLocale(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Every layout in the table has to be one go can actually format and parse
// with, since it goes straight into the config as the answer.
func TestTableLayoutsAreValid(t *testing.T) {
	isolateLocale(t)

	moment := time.Date(2024, time.March, 5, 15, 30, 0, 0, time.UTC)

	for layout := range dateLayoutRegions {
		for _, timeLayout := range []string{"15:04", "3:04 PM"} {
			full := layout + " " + timeLayout

			read, err := time.Parse(full, moment.Format(full))
			if err != nil {
				t.Errorf("layout %q does not round trip: %v", full, err)
				continue
			}
			if !read.Equal(moment) {
				t.Errorf("layout %q read back %s, want %s", full, read, moment)
			}
		}
	}
}

// dayFirstLayout has to agree with the table, since init derives day_first
// from whichever layout the locale produced.
func TestTableLayoutsAgreeWithDayFirst(t *testing.T) {
	dayFirst := map[string]bool{
		"1/2/2006":   false,
		"2/1/2006":   true,
		"2.1.2006":   true,
		"2-1-2006":   true,
		"2006-01-02": false,
		"2006/1/2":   false,
	}

	for layout := range dateLayoutRegions {
		want, known := dayFirst[layout]
		if !known {
			t.Errorf("layout %q is in the table but not in this test", layout)
			continue
		}
		if got := dayFirstLayout(layout); got != want {
			t.Errorf("dayFirstLayout(%q) = %v, want %v", layout, got, want)
		}
	}
}
