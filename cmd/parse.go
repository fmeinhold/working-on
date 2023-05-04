package cmd

import (
	"strings"
	"time"
)

func GuessTypes(args []string) ([]time.Time, []string) {
	var result []time.Time
	var description []string
	today := time.Now()
	for _, arg := range args {

		if strings.Contains(arg, "-") {
			times, _ := GuessTypes(strings.Split(arg, "-"))
			if len(times) != 2 {
				description = append(description, arg)
				continue
			}

			result = append(result, times...)

		} else if strings.Contains(arg, ".") {
			// try different layouts
			layouts := []string{"2.1", "02.01", "2. Jan", "2 Jan", "02.01.2006", "2.1.06"}
			for _, layout := range layouts {
				t, err := time.ParseInLocation(layout, arg, today.Location())
				if err != nil {
					description = append(description, arg)
					continue
				}
				if t.Year() == 0 {
					t = time.Date(today.Year(), t.Month(), t.Day(), 0, 0, 0, 0, today.Location())
				}
				result = append(result, t)
			}
			// Time
		} else if strings.Contains(arg, ":") {
			t, err := time.ParseInLocation("15:04", arg, today.Location())
			if err != nil {
				description = append(description, arg)
				continue
			}
			t = time.Date(today.Year(), today.Month(), today.Day(), t.Hour(), t.Minute(), 0, 0, today.Location())

			result = append(result, t)
		}
	}
	return result, description
}
