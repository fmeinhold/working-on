package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/fefeme/workingon/util"
	"github.com/fefeme/workingon/workingon"

	"github.com/alexeyco/simpletable"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/theckman/yacspin"
)

func NewProjectsCommand(cfg *workingon.Config) *cobra.Command {
	var includeArchived bool

	var projectsCommand = &cobra.Command{
		Use:   "projects",
		Short: "List all projects",
		Long:  `List all projects`,
		RunE: func(cmd *cobra.Command, args []string) error {

			spinner, err := yacspin.New(yacspin.Config{
				Frequency:     100 * time.Millisecond,
				CharSet:       yacspin.CharSets[11],
				Suffix:        " retrieving projects ...",
				StopCharacter: "✓",
				StopColors:    []string{"fgGreen"},
			})

			if err != nil {
				panic(err)
			}

			currentPid := workingon.CurrentProject(cfg)
			currentKey := ""
			if currentPid != 0 {
				currentKey = strconv.Itoa(currentPid)
			}
			var currentName string

			table := simpletable.New()

			table.Header = &simpletable.Header{
				Cells: []*simpletable.Cell{
					{Align: simpletable.AlignLeft, Text: ""},
					{Align: simpletable.AlignLeft, Text: "Key"},
					{Align: simpletable.AlignLeft, Text: "Name"},
				},
			}

			// The status column only earns its place once archived projects can
			// actually appear.
			if includeArchived {
				table.Header.Cells = append(table.Header.Cells,
					&simpletable.Cell{Align: simpletable.AlignLeft, Text: "Status"})
			}

			for _, source := range workingon.Registry.RegisteredSources {
				if len(args) > 0 && !util.StringInSliceI(source.GetName(), args) {
					continue
				}
				spinner.Message(fmt.Sprintf(source.GetName()))
				spinner.Start()
				projects, err := source.GetProjects(includeArchived)
				spinner.Stop()

				if len(projects) > 0 {
					fmt.Println(source.GetName())
				}

				if err != nil {
					return err
				}

				for _, project := range projects {
					selected := currentKey != "" && project.Key == currentKey
					if selected {
						currentName = project.Name
					}

					r := []*simpletable.Cell{
						{Align: simpletable.AlignLeft, Text: marker(selected)},
						{Align: simpletable.AlignLeft, Text: highlight(project.Key, selected)},
						{Align: simpletable.AlignLeft, Text: highlight(project.Name, selected)},
					}

					if includeArchived {
						status := "active"
						if project.Archived {
							status = "archived"
						}
						r = append(r, &simpletable.Cell{
							Align: simpletable.AlignLeft,
							Text:  highlight(status, selected),
						})
					}

					table.Body.Cells = append(table.Body.Cells, r)
				}
				table.SetStyle(simpletable.StyleCompactLite)
				fmt.Println(table.String())
			}
			fmt.Print(currentProjectNote(currentPid, currentName, includeArchived))

			return nil

		},
	}
	projectsCommand.Flags().BoolVarP(&includeArchived, "archived", "a", false,
		"Include archived projects")

	return projectsCommand
}

// selectedColour marks the project a new entry would be filed under. color
// disables itself when stdout is not a terminal, so piped output stays clean.
var selectedColour = color.New(color.FgGreen, color.Bold)

func highlight(text string, selected bool) string {
	if !selected {
		return text
	}
	return selectedColour.Sprint(text)
}

// marker restates the highlight as a character, for the case where the colour
// was suppressed - piped output, or a terminal that has none.
func marker(selected bool) string {
	if !selected {
		return " "
	}
	return selectedColour.Sprint("▸")
}

// currentProjectNote says which project is selected and what chose it, since
// the answer depends on the directory you happen to be standing in - a
// .workingon.yaml beside a checkout sets toggl_default_pid for that checkout.
func currentProjectNote(pid int, name string, listedArchived bool) string {
	if pid == 0 {
		return "\nNo project is currently selected - no toggl_default_pid is set. " +
			"Run `wo init --local` to set one for this repository.\n"
	}

	if name == "" {
		// Selected but absent from the listing: archived, or from another
		// workspace. Saying so beats leaving the marker unexplained.
		hint := " - it is not in the list above"
		if !listedArchived {
			hint += " (try --archived)"
		}
		return fmt.Sprintf("\nCurrent project: %d, from toggl_default_pid%s.\n", pid, hint)
	}

	return fmt.Sprintf("\nCurrent project: %s (%d), from toggl_default_pid.\n",
		selectedColour.Sprint(name), pid)
}
