package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alexeyco/simpletable"
	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
)

func NewShowCommand(cfg *workingon.Config) *cobra.Command {
	var (
		from string
		to   string
		list bool
	)

	showCommand := &cobra.Command{
		Use:   "show [date]",
		Short: "Show everything tracked on a day",
		Long: `Show everything tracked on a day, today unless another one is named.

The day is drawn as half hour slots from 06:00 to 18:00, each entry filling the
time it covers. An early start or a late finish stretches those ends rather than
going unseen - name an end yourself with --from or --to and it holds instead,
with whatever it leaves out accounted for underneath. --list gives the same day
as a table.

The date is read the way every other date in wo is: "today", "yesterday", a
weekday name for the most recent such day, or a date in your configured layout,
shortened to the day or the day and month if you like - with a layout of
"2.1.2006" that makes "6" and "6.8" both valid.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			day := time.Now().In(&cfg.Settings.Location)
			if len(args) > 0 {
				parsed, err := ParseDateFromArg(args[0], cfg)
				if err != nil {
					return err
				}
				day = parsed
			}

			window, err := parseWindow(from, to)
			if err != nil {
				return err
			}

			start := startOfDay(day, &cfg.Settings.Location)
			end := start.AddDate(0, 0, 1)

			cl := toggl.NewToggl(cfg.Settings.ToggleApiToken)
			listed, err := cl.TimeEntries.List(&start, &end)
			if err != nil {
				return err
			}

			entries := entriesStartingOn(start, listed.TimeEntries)
			names := &dayNames{}

			if list {
				fmt.Print(RenderDay(start, entries, cfg, names.names))
				return nil
			}

			fmt.Print(RenderTimeline(start, entries, cfg, window, names.names))

			return nil
		},
	}

	showCommand.Flags().StringVar(&from, "from", "", "Hour the timeline starts at (default 6:00)")
	showCommand.Flags().StringVar(&to, "to", "", "Hour the timeline ends at (default 18:00)")
	showCommand.Flags().BoolVarP(&list, "list", "l", false, "List the entries as a table instead")

	return showCommand
}

// parseWindow reads the --from and --to flags, each falling back to its end of
// the default working day.
func parseWindow(from, to string) (dayWindow, error) {
	window := defaultWindow

	if from != "" {
		at, err := parseClockFlag("from", from)
		if err != nil {
			return window, err
		}
		window.from, window.fixedFrom = at, true
	}

	if to != "" {
		at, err := parseClockFlag("to", to)
		if err != nil {
			return window, err
		}
		window.to, window.fixedTo = at, true
	}

	if window.to.sub(window.from) <= 0 {
		return window, fmt.Errorf("--to must come after --from")
	}

	return window, nil
}

// entriesStartingOn keeps the entries that began on the given day, sorted by
// start time.
//
// The listing is narrowed by the api as well, but where its range ends is not
// something to take on faith, and v9 does not document the order it answers in.
func entriesStartingOn(day time.Time, entries []toggl.TimeEntry) []toggl.TimeEntry {
	next := day.AddDate(0, 0, 1)

	var kept []toggl.TimeEntry
	for _, entry := range entries {
		if entry.Start == nil {
			continue
		}
		start := entry.Start.In(day.Location())
		if start.Before(day) || !start.Before(next) {
			continue
		}
		kept = append(kept, entry)
	}

	sort.Slice(kept, func(i, j int) bool {
		return kept[i].Start.Before(*kept[j].Start)
	})

	return kept
}

// RenderDay lays out a day's entries in the order they were worked, with what
// they add up to underneath.
//
// The project and task columns appear only when something fills them, so a
// workspace that files everything under one project is not asked to read the
// same name on every line.
func RenderDay(day time.Time, entries []toggl.TimeEntry, cfg *workingon.Config,
	resolve func(*toggl.TimeEntry) entryNames) string {

	heading := formatMoment(day, cfg.Settings.DateLayout)

	if len(entries) == 0 {
		return fmt.Sprintf("⏲  Nothing tracked on %s.\n", heading)
	}

	loc := &cfg.Settings.Location
	clock := timeLayout(cfg)

	type row struct {
		start       string
		end         string
		duration    string
		description string
		project     string
		task        string
	}

	var (
		rows       []row
		total      time.Duration
		hasProject bool
		hasTask    bool
		anyRunning bool
	)

	for i := range entries {
		entry := &entries[i]
		names := resolve(entry)
		duration := entryDuration(entry)
		total += duration

		hasProject = hasProject || names.project != ""
		hasTask = hasTask || names.task != ""
		anyRunning = anyRunning || entry.IsRunning()

		rows = append(rows, row{
			start:       entry.Start.In(loc).Format(clock),
			end:         entryEnd(entry, clock, loc),
			duration:    tableDuration(duration),
			description: describedAs(entry.Description),
			project:     names.project,
			task:        names.task,
		})
	}

	table := simpletable.New()
	table.SetStyle(simpletable.StyleCompactLite)

	headings := []string{"Start", "End", "Duration", "Description"}
	if hasProject {
		headings = append(headings, "Project")
	}
	if hasTask {
		headings = append(headings, "Task")
	}

	for _, text := range headings {
		table.Header.Cells = append(table.Header.Cells,
			&simpletable.Cell{Align: simpletable.AlignLeft, Text: text})
	}

	for _, r := range rows {
		values := []string{r.start, r.end, r.duration, r.description}
		if hasProject {
			values = append(values, r.project)
		}
		if hasTask {
			values = append(values, r.task)
		}

		var cells []*simpletable.Cell
		for _, value := range values {
			cells = append(cells, &simpletable.Cell{Align: simpletable.AlignLeft, Text: value})
		}
		table.Body.Cells = append(table.Body.Cells, cells)
	}

	footer := []string{"Total", "", tableDuration(total)}
	for len(footer) < len(headings) {
		footer = append(footer, "")
	}
	for _, text := range footer {
		table.Footer.Cells = append(table.Footer.Cells,
			&simpletable.Cell{Align: simpletable.AlignLeft, Text: text})
	}

	var out strings.Builder
	fmt.Fprintf(&out, "⏲  %s\n\n", heading)
	out.WriteString(table.String())
	out.WriteString("\n")

	if anyRunning {
		out.WriteString("\nA timer is still running, so the total is what it is right now.\n")
	}

	return out.String()
}

// describedAs names an entry that was saved without a description, so that a
// row or a block is never left standing empty.
func describedAs(description string) string {
	if description == "" {
		return "(no description)"
	}
	return description
}

// entryDuration is how long an entry ran for, or has been running so far.
func entryDuration(entry *toggl.TimeEntry) time.Duration {
	if entry.IsRunning() {
		if entry.Start == nil {
			return 0
		}
		return time.Since(*entry.Start)
	}
	return time.Duration(entry.Duration) * time.Second
}

// entryEnd is the clock time an entry stopped at, or "running".
//
// An entry that was saved without a stop is dated from its own duration rather
// than left blank, since that is the one number it is certain to carry.
func entryEnd(entry *toggl.TimeEntry, layout string, loc *time.Location) string {
	if entry.IsRunning() {
		return "running"
	}
	if entry.Stop != nil {
		return entry.Stop.In(loc).Format(layout)
	}
	if entry.Start == nil {
		return ""
	}
	return entry.Start.Add(entryDuration(entry)).In(loc).Format(layout)
}

// tableDuration is humanDuration with the sub-minute case shortened, since a
// column is no place for a sentence.
func tableDuration(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	return humanDuration(d)
}

// timeLayout is the time of day half of the configured date and time layout.
// The date itself is in the heading, so repeating it on every row would only
// cost width.
func timeLayout(cfg *workingon.Config) string {
	rest := strings.TrimPrefix(cfg.Settings.DateTimeLayout, cfg.Settings.DateLayout)
	rest = strings.TrimSpace(rest)

	if rest == "" || rest == cfg.Settings.DateTimeLayout {
		return "15:04"
	}

	return rest
}

// dayNames resolves the project and task behind a listing's ids, indexing the
// projects once however many entries ask: the listing is a request, and a day
// can hold a good many entries.
type dayNames struct {
	projects map[int]string
}

func (d *dayNames) names(entry *toggl.TimeEntry) entryNames {
	var names entryNames

	if entry.TaskId != 0 {
		if task, err := workingon.Registry.GetTask(strconv.Itoa(entry.TaskId)); err == nil && task != nil {
			names.task = task.Summary
			names.project = task.Project.Name
		}
		if names.task == "" {
			names.task = fmt.Sprintf("#%d", entry.TaskId)
		}
	}

	if names.project == "" && entry.ProjectId != 0 {
		names.project = d.project(entry.ProjectId)
	}

	return names
}

func (d *dayNames) project(projectId int) string {
	if d.projects == nil {
		d.projects = projectNames()
	}

	if name, exists := d.projects[projectId]; exists {
		return name
	}

	return fmt.Sprintf("#%d", projectId)
}

// projectNames indexes every project of every source by id, archived ones
// included - a day being read back is as likely to be an old one as today.
func projectNames() map[int]string {
	names := make(map[int]string)

	for projectId, project := range projectIndex() {
		names[projectId] = project.Name
	}

	return names
}

// projectIndex is every project of every source by id, archived ones included.
// A caller that cares which is which reads Archived; one that does not gets a
// name either way.
func projectIndex() map[int]workingon.Project {
	index := make(map[int]workingon.Project)

	for _, source := range workingon.Registry.RegisteredSources {
		projects, err := source.GetProjects(true)
		if err != nil {
			continue
		}
		for _, project := range projects {
			projectId, err := strconv.Atoi(project.Key)
			if err != nil {
				continue
			}
			index[projectId] = project
		}
	}

	return index
}
