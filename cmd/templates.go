package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fefeme/workingon/workingon"

	"github.com/spf13/cobra"
	"github.com/theckman/yacspin"
)

func NewTemplatesCommand(cfg *workingon.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "templates",
		Short: "List the time entry templates",
		Long: `List the time entry templates set in your config.

Each is booked by its alias - ` + "`wo add ds`" + ` or ` + "`wo start ds`" + ` - and a description
carrying {{.placeholders}} is filled from --templateArgs, or by being asked for
what no argument answered.

The project and task a template pins are named where they can be looked up, and
left as ids where they cannot.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			labels := lookupTemplateLabels(cfg.Templates)

			if jsonOutput {
				return emitTemplates(cfg.Templates, labels)
			}

			fmt.Print(renderTemplates(cfg.Templates, labels))
			return nil
		},
	}
}

type templateLabels struct {
	projects map[int]string
	tasks    map[int]string
}

// It costs a round trip, so it is skipped entirely for templates that pin
// nothing, and a lookup that fails leaves the id to stand for itself rather
// than failing a listing that is otherwise readable straight from the config.
func lookupTemplateLabels(templates []workingon.TemplateConfig) templateLabels {
	labels := templateLabels{projects: map[int]string{}, tasks: map[int]string{}}

	var projectIds, taskIds []int
	for _, template := range templates {
		if template.TogglPid != 0 {
			projectIds = append(projectIds, template.TogglPid)
		}
		if template.TogglTask != 0 {
			taskIds = append(taskIds, template.TogglTask)
		}
	}

	if len(projectIds) == 0 && len(taskIds) == 0 {
		return labels
	}

	spinner, err := yacspin.New(yacspin.Config{
		Frequency:     100 * time.Millisecond,
		CharSet:       yacspin.CharSets[11],
		Suffix:        " naming projects and tasks ...",
		StopCharacter: "✓",
		StopColors:    []string{"fgGreen"},
	})

	// No spinner where the output is a document: it writes to the same stream,
	// and would land in the middle of it.
	spin := err == nil && !jsonOutput
	if spin {
		spinner.Start()
	}

	if len(projectIds) > 0 {
		for id, project := range projectIndex() {
			labels.projects[id] = project.Name
		}
	}

	for _, id := range taskIds {
		if task, err := workingon.Registry.GetTask(strconv.Itoa(id)); err == nil && task != nil {
			labels.tasks[id] = task.Summary
		}
	}

	if spin {
		spinner.Stop()
	}

	return labels
}

// renderTemplates lays the templates out the way `wo tasks` lays out tasks:
// the aliases in a column of their own, since the alias is what you type, and
// everything a template pins trailing the description it belongs to.
//
// Config order is kept rather than sorted - that is the order they were
// written in, and re-arranging someone's own list helps nobody.
// emitTemplates answers with the aliases that can be booked, and what each one
// stands for.
//
// The start and stop are left as the times of day the config gives, since that
// is what they are - a template is not dated until it is booked.
func emitTemplates(templates []workingon.TemplateConfig, labels templateLabels) error {
	type templateJSON struct {
		Alias       string `json:"alias"`
		Description string `json:"description"`
		Project     *ref   `json:"project"`
		Task        *ref   `json:"task"`
		Start       string `json:"start,omitempty"`
		Stop        string `json:"stop,omitempty"`
	}

	listed := make([]templateJSON, 0, len(templates))

	for _, template := range templates {
		listed = append(listed, templateJSON{
			Alias:       template.Alias,
			Description: template.Description,
			Project:     named(template.TogglPid, labels.projects[template.TogglPid]),
			Task:        named(template.TogglTask, labels.tasks[template.TogglTask]),
			Start:       template.Start,
			Stop:        template.Stop,
		})
	}

	return emit(struct {
		Templates []templateJSON `json:"templates"`
	}{listed})
}

func renderTemplates(templates []workingon.TemplateConfig, labels templateLabels) string {
	if len(templates) == 0 {
		return "\nNo templates configured. Add them under `templates:` in your config " +
			"to book an entry you repeat by a short alias - see config.example.yaml.\n"
	}

	var out strings.Builder

	fmt.Fprintf(&out, "\n%s\n\n", taskNoteColour.Sprint(countOfTemplates(len(templates))))

	details := make([]string, len(templates))
	for i, template := range templates {
		details[i] = templateDetail(template, labels)
	}

	aliasWidth := 0
	descriptionWidth := 0
	for i, template := range templates {
		if width := len([]rune(template.Alias)); width > aliasWidth {
			aliasWidth = width
		}
		// Only a description with something after it has to line up with the
		// rest; one that ends its row can overhang without disturbing them.
		if width := len([]rune(template.Description)); details[i] != "" && width > descriptionWidth {
			descriptionWidth = width
		}
	}

	for i, template := range templates {
		fmt.Fprintf(&out, "  %s  ", taskKeyColour.Sprintf("%-*s", aliasWidth, template.Alias))

		if details[i] == "" {
			fmt.Fprintf(&out, "%s\n", template.Description)
			continue
		}

		fmt.Fprintf(&out, "%-*s  %s\n", descriptionWidth, template.Description,
			taskNoteColour.Sprint("· "+details[i]))
	}

	fmt.Fprintf(&out, "\n%s\n", taskNoteColour.Sprint("Book one with `wo add <alias>` or `wo start <alias>`"))

	return out.String()
}

// templateDetail is what a template pins, said after its description: the
// project, the task, and the times, each only where it is set.
func templateDetail(template workingon.TemplateConfig, labels templateLabels) string {
	var parts []string

	if template.TogglPid != 0 {
		parts = append(parts, labelFor(labels.projects, template.TogglPid))
	}
	if template.TogglTask != 0 {
		parts = append(parts, labelFor(labels.tasks, template.TogglTask))
	}
	if when := templateWhen(template); when != "" {
		parts = append(parts, when)
	}

	return strings.Join(parts, " · ")
}

// labelFor names an id, falling back to the id itself when the lookup had no
// answer - offline, or a project that has since been deleted.
func labelFor(labels map[int]string, id int) string {
	if name, known := labels[id]; known && name != "" {
		return name
	}
	return "#" + strconv.Itoa(id)
}

// templateWhen is the span a template books, for one that fixes its own times.
// A template that sets only one end is worth showing as such: the other is
// taken from the command line.
func templateWhen(template workingon.TemplateConfig) string {
	switch {
	case template.Start != "" && template.Stop != "":
		return template.Start + "-" + template.Stop
	case template.Start != "":
		return "from " + template.Start
	case template.Stop != "":
		return "until " + template.Stop
	}
	return ""
}

func countOfTemplates(count int) string {
	if count == 1 {
		return "1 template"
	}
	return fmt.Sprintf("%d templates", count)
}
