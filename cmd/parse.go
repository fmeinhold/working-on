package cmd

import (
	"fmt"
	"github.com/fmeinhold/workingon/toggl_api"
	"strings"
	"time"
)

func parseTime(arg string) *time.Time {
	today := time.Now()

	t, err := time.ParseInLocation("15:04", arg, today.Location())
	if err != nil {
		return nil
	}
	t = time.Date(today.Year(), today.Month(), today.Day(), t.Hour(), t.Minute(), 0, 0, today.Location())
	return &t
}

func newTimeEntryFromArgs(args []string) (*toggl_api.TimeEntry, error) {
	timeEntry := &toggl_api.TimeEntry{}

	var description []string

	today := time.Now()
	for _, arg := range args {

		duration, err := time.ParseDuration(arg)
		if err == nil {
			fmt.Println("duration detected", duration, duration.Seconds())
			timeEntry.Duration = int64(duration.Seconds())
			continue
		}

		if strings.Contains(arg, "-") {
			val := strings.Split(arg, "-")
			if len(val) != 2 {
				description = append(description, arg)
				continue
			}
			from := parseTime(val[0])
			to := parseTime(val[1])

			if from != nil && to != nil {
				timeEntry.Start = from
				timeEntry.Duration = int64(to.Sub(*from).Seconds())
			} else {
				description = append(description, arg)
				continue
			}

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
				timeEntry.Start = &t
			}
			// Time
		} else if strings.Contains(arg, ":") {
			timeEntry.Start = parseTime(arg)
			timeEntry.Duration = timeEntry.Start.Unix()
		} else {
			description = append(description, arg)
		}
	}

	if timeEntry.Start == nil || timeEntry.Start.IsZero() {
		now := time.Now()
		timeEntry.Start = &now
		timeEntry.Duration = now.Unix()
	}
	timeEntry.Description = strings.Join(description, " ")

	return timeEntry, nil
}
