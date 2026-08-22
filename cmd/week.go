package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/alexeyco/simpletable"
	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"
)

// projectsWidth is as much of a day's projects as a row will carry. The hours
// are what this listing is about, and a week of full project names would push
// them off the side of the terminal.
const projectsWidth = 44

// weekStart is the Monday of the week a day falls in.
//
// Wo reads a week as Monday to Sunday: it is the week work is planned and
// billed in, whichever day the calendar on the wall begins with.
func weekStart(day time.Time, loc *time.Location) time.Time {
	start := startOfDay(day, loc)

	// Sunday is the last day of the week here, not the first, so it counts as
	// six days along rather than none.
	return startOfDay(start.AddDate(0, 0, -((int(start.Weekday())+6)%7)), loc)
}

// dayTotal is one day of the week as the summary reads it.
type dayTotal struct {
	Day      time.Time
	Entries  int
	Tracked  time.Duration
	Projects []string
	Running  bool
}

// weekOf places a week's entries on the days they were started on, which is
// the day wo files an entry under everywhere else. Anything that began outside
// the week is left out rather than folded into an end of it.
func weekOf(start time.Time, entries []toggl.TimeEntry,
	resolve func(*toggl.TimeEntry) entryNames) []dayTotal {

	loc := start.Location()

	week := make([]dayTotal, 7)
	onDate := make(map[string]int, 7)
	for i := range week {
		week[i].Day = startOfDay(start.AddDate(0, 0, i), loc)

		// Dates rather than a count of hours since the start: a week with the
		// clocks going forward in it does not hold seven equal days.
		onDate[week[i].Day.Format("2006-01-02")] = i
	}

	for i := range entries {
		entry := &entries[i]
		if entry.Start == nil {
			continue
		}

		index, within := onDate[entry.Start.In(loc).Format("2006-01-02")]
		if !within {
			continue
		}

		day := &week[index]
		day.Entries++
		day.Tracked += entryDuration(entry)
		day.Running = day.Running || entry.IsRunning()

		if project := resolve(entry).project; project != "" {
			day.Projects = appendOnce(day.Projects, project)
		}
	}

	return week
}

// appendOnce keeps the order the projects were met in, which is the order the
// day happened in - a set would hand back a different week every time.
func appendOnce(names []string, name string) []string {
	for _, already := range names {
		if already == name {
			return names
		}
	}

	return append(names, name)
}

// RenderWeek is the week as a day per row. Days with nothing on them are rows
// too: a blank Wednesday is something you want to see.
func RenderWeek(week []dayTotal, cfg *workingon.Config) string {
	first, last := week[0], week[len(week)-1]

	from := first.Day.Format(dateLayout(cfg))
	until := last.Day.Format(dateLayout(cfg))

	var (
		total       time.Duration
		entries     int
		anyRunning  bool
		hasProjects bool
	)
	for _, day := range week {
		total += day.Tracked
		entries += day.Entries
		anyRunning = anyRunning || day.Running
		hasProjects = hasProjects || len(day.Projects) > 0
	}

	if entries == 0 {
		return fmt.Sprintf("⏲  Nothing tracked between %s and %s.\n", from, until)
	}

	table := simpletable.New()
	table.SetStyle(simpletable.StyleCompactLite)

	headings := []string{"Day", "Date", "Entries", "Tracked"}
	if hasProjects {
		headings = append(headings, "Projects")
	}

	for _, text := range headings {
		table.Header.Cells = append(table.Header.Cells,
			&simpletable.Cell{Align: simpletable.AlignLeft, Text: text})
	}

	for _, day := range week {
		values := []string{
			day.Day.Format("Monday"),
			day.Day.Format(dateLayout(cfg)),
			countOrNothing(day.Entries),
			trackedOrNothing(day.Tracked, day.Entries),
		}
		if hasProjects {
			values = append(values, shortened(strings.Join(day.Projects, ", "), projectsWidth))
		}

		var cells []*simpletable.Cell
		for _, value := range values {
			cells = append(cells, &simpletable.Cell{Align: simpletable.AlignLeft, Text: value})
		}
		table.Body.Cells = append(table.Body.Cells, cells)
	}

	footer := []string{"", "Total", strconv.Itoa(entries), tableDuration(total)}
	for len(footer) < len(headings) {
		footer = append(footer, "")
	}
	for _, text := range footer {
		table.Footer.Cells = append(table.Footer.Cells,
			&simpletable.Cell{Align: simpletable.AlignLeft, Text: text})
	}

	var out strings.Builder
	fmt.Fprintf(&out, "⏲  %s to %s\n\n", from, until)
	out.WriteString(table.String())
	out.WriteString("\n")

	if anyRunning {
		out.WriteString("\nA timer is still running, so the total is what it is right now.\n")
	}

	return out.String()
}

// A day nobody worked reads better as a dash than as a nought: none of these
// rows is a measurement of zero, they are days there is nothing to report on.
func countOrNothing(entries int) string {
	if entries == 0 {
		return "-"
	}
	return strconv.Itoa(entries)
}

func trackedOrNothing(tracked time.Duration, entries int) string {
	if entries == 0 {
		return "-"
	}
	return tableDuration(tracked)
}

func dateLayout(cfg *workingon.Config) string {
	if cfg.Settings.DateLayout == "" {
		return "2.1.2006"
	}
	return cfg.Settings.DateLayout
}
