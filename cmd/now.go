package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
)

func NewNowCommand(cfg *workingon.Config) *cobra.Command {
	nowCommand := &cobra.Command{
		Use:   "now",
		Short: "Show the timer that is running right now",
		Long:  `Show the timer that is running right now.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := toggl.NewToggl(cfg.Settings.ToggleApiToken)

			current, err := cl.TimeEntries.Current()
			if err != nil {
				return err
			}

			if jsonOutput {
				return emitCurrent(current, cfg)
			}

			prompt, _ := cmd.Flags().GetBool("prompt")
			fmt.Print(RenderCurrent(current, cfg, prompt))

			return nil
		},
	}
	nowCommand.Flags().BoolP("prompt", "p", false, "Output an indicator for usage in a shell prompt")

	return nowCommand
}

// RenderCurrent describes the running time entry, or the lack of one.
//
// A shell prompt has room for a marker and nothing else, so that mode stays
// terse; everywhere else the entry is spelled out in full.
func RenderCurrent(entry *toggl.TimeEntry, cfg *workingon.Config, prompt bool) string {
	if prompt {
		return "⏲ \n"
	}

	if entry == nil {
		return "⏲  Nothing is running - you are slacking off. Go back to work!\n"
	}

	loc := &cfg.Settings.Location
	names := nameResolver(entry)

	var out strings.Builder
	out.WriteString("⏲  Currently working on\n\n")

	// Tabwriter keeps the labels and values in two columns however long the
	// resolved names turn out to be.
	table := tabwriter.NewWriter(&out, 0, 0, 3, ' ', 0)

	row := func(label, value string) {
		if value == "" {
			return
		}
		fmt.Fprintf(table, "   %s\t%s\n", label, value)
	}

	row("Description", entry.Description)
	row("Project", names.project)
	row("Task", names.task)

	if entry.Start != nil {
		start := entry.Start.In(loc)
		row("Started", formatMoment(start, cfg.Settings.DateTimeLayout))
		row("Elapsed", humanDuration(time.Since(start)))
	}

	table.Flush()

	return out.String()
}

// entryNames are the human readable names behind a time entry's ids.
type entryNames struct {
	project string
	task    string
}

// nameResolver is the lookup RenderCurrent uses, as a variable so a test can
// exercise the layout without reaching for the network.
var nameResolver = resolveNames

// resolveNames turns the project and task ids on an entry into names.
//
// A task carries its project with it, so resolving the task usually answers
// both from the local cache without a request. An id that cannot be named is
// shown as an id rather than dropped, so the output never hides where an entry
// was filed - reporting a lookup failure would only be noise in a command whose
// whole job is a one line status.
func resolveNames(entry *toggl.TimeEntry) entryNames {
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
		names.project = lookupProjectName(entry.ProjectId)
		if names.project == "" {
			names.project = fmt.Sprintf("#%d", entry.ProjectId)
		}
	}

	return names
}

// lookupProjectName searches the configured sources for a project id, returning
// an empty string if none of them knows it.
//
// Active projects are searched first and archived ones only if that misses: a
// running timer almost always names a live project, and the archived listing is
// an order of magnitude larger in a workspace with any history.
func lookupProjectName(projectId int) string {
	for _, includeArchived := range []bool{false, true} {
		if name := findProjectName(projectId, includeArchived); name != "" {
			return name
		}
	}

	return ""
}

func findProjectName(projectId int, includeArchived bool) string {
	key := strconv.Itoa(projectId)

	for _, source := range workingon.Registry.RegisteredSources {
		projects, err := source.GetProjects(includeArchived)
		if err != nil {
			continue
		}
		for _, project := range projects {
			if project.Key == key {
				return project.Name
			}
		}
	}

	return ""
}

// formatMoment renders a local time using the configured layout, prefixed with
// the weekday unless the layout already asks for one.
func formatMoment(moment time.Time, layout string) string {
	if layout == "" {
		layout = "2.1.2006 15:04"
	}
	if strings.Contains(layout, "Mon") {
		return moment.Format(layout)
	}
	return moment.Format("Monday, " + layout)
}

// humanDuration renders a duration the way it would be said out loud, rather
// than as Go's "2h31m12.7s".
func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return "less than a minute"
	}

	d = d.Round(time.Minute)
	hours := int(d / time.Hour)
	minutes := int((d % time.Hour) / time.Minute)

	switch {
	case hours == 0:
		return fmt.Sprintf("%dm", minutes)
	case minutes == 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
}
