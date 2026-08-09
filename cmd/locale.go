package cmd

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// The date and time defaults `wo init` offers come from the machine's locale.
// Someone in Chicago should not have to correct a German date on their first
// run, and the regions differ on all three of the order, the separator and
// whether the clock goes to 12 or to 24.
//
// This is a table rather than CLDR data: a handful of layouts covers the
// regions this is likely to run in, the answer is only a default the prompt
// invites you to correct, and a date library is a lot to carry for two strings.

// dateLayoutRegions groups the regions that write a date the same way. The
// layouts are Go's reference time - https://pkg.go.dev/time#pkg-constants.
var dateLayoutRegions = map[string][]string{
	// Month first.
	"1/2/2006": {"US", "PH"},

	// Day first, on slashes.
	"2/1/2006": {
		"GB", "IE", "AU", "NZ", "IN", "ZA", "ES", "FR", "IT", "PT", "BR",
		"GR", "ID", "TH", "VN", "MY", "SG", "AR", "CL", "CO", "MX", "PE",
	},

	// Day first, on dots.
	"2.1.2006": {
		"DE", "AT", "CH", "DK", "NO", "FI", "IS", "PL", "CZ", "SK", "RU",
		"UA", "TR", "RO", "BG", "HR", "SI", "RS", "EE", "LV",
	},

	// Day first, on dashes.
	"2-1-2006": {"NL", "BE"},

	// Year first.
	"2006-01-02": {"SE", "LT", "HU", "CA", "KR"},
	"2006/1/2":   {"JP", "CN", "TW", "HK"},
}

// regionDateLayouts is dateLayoutRegions read the way it gets used.
var regionDateLayouts = func() map[string]string {
	byRegion := make(map[string]string)
	for layout, regions := range dateLayoutRegions {
		for _, region := range regions {
			byRegion[region] = layout
		}
	}
	return byRegion
}()

// hour12Regions write the time of day on a 12 hour clock.
var hour12Regions = map[string]bool{
	"US": true, "PH": true, "CA": true, "AU": true, "NZ": true, "IN": true,
	"PK": true, "BD": true, "MY": true, "EG": true, "SA": true, "MX": true,
	"CO": true,
}

// languageRegions is where a locale naming no region is taken to be, so a bare
// "de" still gets German dates. "en" goes to the US the way CLDR has it.
var languageRegions = map[string]string{
	"en": "US", "de": "DE", "fr": "FR", "es": "ES", "it": "IT", "pt": "PT",
	"nl": "NL", "sv": "SE", "da": "DK", "nb": "NO", "no": "NO", "fi": "FI",
	"is": "IS", "pl": "PL", "cs": "CZ", "sk": "SK", "ru": "RU", "uk": "UA",
	"tr": "TR", "ro": "RO", "bg": "BG", "hr": "HR", "sl": "SI", "sr": "RS",
	"et": "EE", "lv": "LV", "lt": "LT", "hu": "HU", "el": "GR", "id": "ID",
	"th": "TH", "vi": "VN", "ja": "JP", "zh": "CN", "ko": "KR", "hi": "IN",
}

// What a machine that will not say where it is gets. US, since a locale that
// went unread is most often one that was never set, and the two are answered
// together so the fallback is one convention rather than a mixture of two.
const (
	fallbackDateLayout = "1/2/2006"
	fallbackTimeLayout = "3:04 PM"
)

func localeDateLayout() string {
	if layout, ok := regionDateLayouts[localeRegion()]; ok {
		return layout
	}
	return fallbackDateLayout
}

// A region the date table does not name is the same unknown region here, so
// the two questions fall back together rather than offering a US date beside
// a European clock.
func localeTimeLayout() string {
	region := localeRegion()

	if _, known := regionDateLayouts[region]; !known {
		return fallbackTimeLayout
	}
	if hour12Regions[region] {
		return "3:04 PM"
	}

	return "15:04"
}

// localeRegion is the region this machine is set to, as an upper case code
// like "DE", or "" where the locale says nothing usable.
func localeRegion() string {
	locale := currentLocale()
	if locale == "" {
		return ""
	}

	language, region, split := strings.Cut(locale, "_")
	if split && region != "" {
		return strings.ToUpper(region)
	}

	return languageRegions[strings.ToLower(language)]
}

// currentLocale reads the locale as "language_REGION", stripped of the
// character set and any modifier - "de_DE.UTF-8@euro" is "de_DE".
//
// The environment is asked in the order POSIX gives it: LC_ALL overrides
// everything, LC_TIME is the category this actually cares about, and LANG is
// the fallback for both.
func currentLocale() string {
	for _, name := range []string{"LC_ALL", "LC_TIME", "LANG"} {
		if locale := normalizeLocale(os.Getenv(name)); locale != "" {
			return locale
		}
	}

	return normalizeLocale(appleLocale())
}

func normalizeLocale(value string) string {
	value = strings.TrimSpace(value)
	value, _, _ = strings.Cut(value, ".")
	value, _, _ = strings.Cut(value, "@")
	value = strings.ReplaceAll(value, "-", "_")

	// The C locale is the absence of a locale rather than a place.
	switch strings.ToUpper(value) {
	case "", "C", "POSIX":
		return ""
	}

	return value
}

// appleLocale is macOS's own setting, for a terminal that was never told to
// export the locale into the environment - which is a setting you can turn
// off, and Terminal.app offers to.
//
// It is a variable so a test can answer for it without an exec.
var appleLocale = func() string {
	if runtime.GOOS != "darwin" {
		return ""
	}

	out, err := exec.Command("defaults", "read", "-g", "AppleLocale").Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}
