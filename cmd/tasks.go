package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
	"github.com/theckman/yacspin"
)

// taskFilter is the project a listing was narrowed to, and what chose it.
// A zero project is the whole workspace.
type taskFilter struct {
	projectId       int
	includeArchived bool

	// projects is the lookup resolved fills in, and nil until it does.
	projects map[int]workingon.Project
}

func (f taskFilter) active() bool { return f.projectId != 0 }

// name is what to call the filtered project. The project's own name is what
// `wo projects` shows, so it is the one to say; the id stands in when the name
// could not be looked up.
func (f taskFilter) name() string {
	if project, known := f.projects[f.projectId]; known && project.Name != "" {
		return project.Name
	}
	return fmt.Sprintf("#%d", f.projectId)
}

// resolved is the filter with the projects it needs to hand: to name the one a
// listing was narrowed to, and to tell a live project from an archived one.
// Left to the caller rather than done on demand, since it costs a round trip.
func (f taskFilter) resolved() taskFilter {
	spinner, err := yacspin.New(yacspin.Config{
		Frequency:     100 * time.Millisecond,
		CharSet:       yacspin.CharSets[11],
		Suffix:        " retrieving projects ...",
		StopCharacter: "✓",
		StopColors:    []string{"fgGreen"},
	})
	if err == nil {
		spinner.Start()
	}

	f.projects = projectIndex()

	if err == nil {
		spinner.Stop()
	}

	return f
}

// archived reports whether a project is one `wo projects` would leave out.
//
// A project the lookup does not know about counts as live: the index is best
// effort, and a listing that quietly dropped every task because the projects
// could not be fetched would be worse than one that shows an archived few.
func (f taskFilter) archived(projectId int) bool {
	project, known := f.projects[projectId]
	return known && project.Archived
}

// apply narrows a source's tasks to the ones the listing is for, counting what
// an archived project cost so the listing can own up to it.
func (f taskFilter) apply(tasks []workingon.Task) *taskListing {
	listing := &taskListing{}

	for _, task := range tasks {
		if f.active() && task.Project.TogglProject != f.projectId {
			continue
		}

		if !f.includeArchived && f.archived(task.Project.TogglProject) {
			listing.hidden++
			continue
		}

		listing.tasks = append(listing.tasks, task)
	}

	return listing
}

// taskListing is what one source had to show, and what it left out.
type taskListing struct {
	tasks  []workingon.Task
	hidden int
}

var (
	taskKeyColour     = color.New(color.FgCyan)
	taskProjectColour = color.New(color.Bold)
	taskNoteColour    = color.New(color.Faint)
)

// loadTasks fetches a source's tasks and narrows them to what the listing is
// for.
func loadTasks(source workingon.Source, filter taskFilter) (*taskListing, error) {
	spinner, err := yacspin.New(yacspin.Config{
		Frequency:     100 * time.Millisecond,
		CharSet:       yacspin.CharSets[11],
		Suffix:        fmt.Sprintf(" %s: retrieving tasks ...", source.GetName()),
		StopCharacter: "✓",
		StopColors:    []string{"fgGreen"},
	})
	if err != nil {
		return nil, err
	}

	spinner.Start()
	tasks, err := source.GetTasks()
	spinner.Stop()
	if err != nil {
		return nil, err
	}

	return filter.apply(tasks), nil
}

// taskGroup is the tasks of one project, under the name to head them with.
type taskGroup struct {
	label string
	tasks []workingon.Task
}

// groupByProject gathers a whole-workspace listing under its projects, since a
// project column repeating the same id down a screenful of rows is noise the
// heading says once. Groups are ordered by name so the same listing reads the
// same way twice.
func groupByProject(tasks []workingon.Task, projects map[int]workingon.Project) []taskGroup {
	order := make(map[string]int)
	var groups []taskGroup

	for _, task := range tasks {
		label := projectLabel(task.Project, projects)

		at, seen := order[label]
		if !seen {
			at = len(groups)
			order[label] = at
			groups = append(groups, taskGroup{label: label})
		}

		groups[at].tasks = append(groups[at].tasks, task)
	}

	sort.Slice(groups, func(i, j int) bool { return groups[i].label < groups[j].label })

	return groups
}

// projectLabel names a project, falling back to its key when the name is not
// to hand - the cached tasks carry ids alone.
func projectLabel(project workingon.Project, projects map[int]workingon.Project) string {
	if known, exists := projects[project.TogglProject]; exists && known.Name != "" {
		return known.Name
	}
	if project.Name != "" {
		return project.Name
	}
	return "#" + project.Key
}

// renderTasks lays a source's tasks out as an indented list rather than a
// bordered table: the keys line up in a column of their own, and everything
// that would have repeated on every row is said once above them.
func renderTasks(source workingon.Source, listing *taskListing, filter taskFilter) string {
	var out strings.Builder

	out.WriteString(taskHeading(source, listing, filter))

	keyWidth := 0
	for _, task := range listing.tasks {
		if width := len([]rune(task.Key)); width > keyWidth {
			keyWidth = width
		}
	}

	if filter.active() {
		writeTaskRows(&out, listing.tasks, "  ", keyWidth)
		return out.String()
	}

	for i, group := range groupByProject(listing.tasks, filter.projects) {
		if i > 0 {
			out.WriteString("\n")
		}
		fmt.Fprintf(&out, "  %s %s\n",
			taskProjectColour.Sprint(group.label),
			taskNoteColour.Sprint("· "+countOfTasks(len(group.tasks))))
		writeTaskRows(&out, group.tasks, "    ", keyWidth)
	}

	return out.String()
}

func writeTaskRows(out *strings.Builder, tasks []workingon.Task, indent string, keyWidth int) {
	for _, task := range tasks {
		fmt.Fprintf(out, "%s%s  %s\n",
			indent, taskKeyColour.Sprintf("%-*s", keyWidth, task.Key), task.Summary)
	}
}

// taskHeading says what the listing covers - which source, how many tasks, and
// the project they were narrowed to, if any.
func taskHeading(source workingon.Source, listing *taskListing, filter taskFilter) string {
	count := countOfTasks(len(listing.tasks))
	if filter.active() {
		count += " in " + filter.name()
	}

	parts := []string{count}

	// The source is only worth naming when there is more than one it could
	// have been.
	if len(workingon.Registry.RegisteredSources) > 1 {
		parts = append([]string{source.GetName()}, parts...)
	}

	if listing.hidden > 0 {
		parts = append(parts, fmt.Sprintf("%d archived (--archived)", listing.hidden))
	}

	return "\n" + taskNoteColour.Sprint(strings.Join(parts, " · ")) + "\n\n"
}

func countOfTasks(count int) string {
	if count == 1 {
		return "1 task"
	}
	return fmt.Sprintf("%d tasks", count)
}

func areNotShown(count int) string {
	if count == 1 {
		return "1 task is"
	}
	return fmt.Sprintf("%d tasks are", count)
}

// taskProjectFilter is the project to narrow a task listing to: none when
// --all is given, otherwise the project an entry started here would land in.
func taskProjectFilter(cmd *cobra.Command, cfg *workingon.Config) taskFilter {
	archived, _ := cmd.Flags().GetBool("archived")

	if all, _ := cmd.Flags().GetBool("all"); all {
		return taskFilter{includeArchived: archived}
	}

	return taskFilter{projectId: workingon.CurrentProject(cfg), includeArchived: archived}
}

func reportTasks(source workingon.Source, listing *taskListing, filter taskFilter) {
	if len(listing.tasks) > 0 {
		fmt.Print(renderTasks(source, listing, filter))
		return
	}

	// Everything this listing had was in an archived project. Saying so beats
	// "no tasks", which reads as a project that has none at all.
	if listing.hidden > 0 {
		if filter.active() {
			fmt.Printf("\n%s is archived, and its %s not shown. Use --archived to list them.\n",
				filter.name(), areNotShown(listing.hidden))
			return
		}
		fmt.Printf("\nNo tasks in an active project - %s in archived ones. Use --archived to list them.\n",
			countOfTasks(listing.hidden))
		return
	}

	if filter.active() {
		fmt.Printf("\nNo tasks in %s. Use --all to list the whole workspace.\n", filter.name())
		return
	}
	fmt.Println("\nNo tasks found.")
}

func initConfigTasks(tasksCommand *cobra.Command, cfg *workingon.Config) {
	for i := range workingon.Registry.RegisteredSources {
		source := workingon.Registry.RegisteredSources[i]
		tasksCommand.AddCommand(&cobra.Command{
			Use:   strings.ToLower(source.GetName()),
			Short: fmt.Sprintf("Get tasks from %s", source.GetName()),
			RunE: func(cmd *cobra.Command, args []string) error {
				if refresh, _ := cmd.Flags().GetBool("refresh"); refresh {
					if err := refreshCache(source); err != nil {
						return err
					}
				}

				filter := taskProjectFilter(cmd, cfg).resolved()

				listing, err := loadTasks(source, filter)
				if err != nil {
					return err
				}

				reportTasks(source, listing, filter)
				return nil
			},
		})
	}
}

func NewTasksCommand(cfg *workingon.Config) *cobra.Command {
	var tasksCommand = &cobra.Command{
		Use:   "tasks",
		Short: "List the tasks for this project",
		Long: `List the tasks for this project.

Narrowed to the project a new entry would be filed under - the one ` + "`wo projects`" + `
marks as current, whether that came from a .workingon.yaml beside your checkout
or the global default. Use --all for the whole workspace, which is listed under
the project each task belongs to.

Tasks of archived projects are left out, as they are from ` + "`wo projects`" + `; --archived
lists them too.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			refresh, _ := cmd.Flags().GetBool("refresh")
			filter := taskProjectFilter(cmd, cfg).resolved()

			for _, source := range workingon.Registry.RegisteredSources {
				if refresh {
					if err := refreshCache(source); err != nil {
						return err
					}
				}

				listing, err := loadTasks(source, filter)
				if err != nil {
					return err
				}

				reportTasks(source, listing, filter)
			}

			return nil
		},
	}
	tasksCommand.PersistentFlags().BoolP("refresh", "r", false,
		"Rebuild the local task cache before listing")
	tasksCommand.PersistentFlags().BoolP("all", "a", false,
		"List every task in the workspace, not just this project's")
	// No shorthand: -a is already --all here, whereas `wo projects` has it for
	// this flag. Better to leave it off than to have -a mean two things.
	tasksCommand.PersistentFlags().Bool("archived", false,
		"Include tasks of archived projects")

	initConfigTasks(tasksCommand, cfg)
	return tasksCommand
}
