package cmd

import (
	"fmt"
	"github.com/alexeyco/simpletable"
	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
	"github.com/theckman/yacspin"
	"strings"
	"time"
)

// loadTasks renders a source's tasks, keeping only those in projectId when it
// is non-zero.
func loadTasks(source workingon.Source, projectId int) (*simpletable.Table, error) {
	table := simpletable.New()

	table.Header = &simpletable.Header{
		Cells: []*simpletable.Cell{
			{Align: simpletable.AlignLeft, Text: "Source"},
			{Align: simpletable.AlignLeft, Text: "Key"},
			{Align: simpletable.AlignLeft, Text: "Summary"},
			{Align: simpletable.AlignLeft, Text: "Project"},
		},
	}

	cfg := yacspin.Config{
		Frequency:     100 * time.Millisecond,
		CharSet:       yacspin.CharSets[11],
		Suffix:        " retrieving tasks ...",
		StopCharacter: "✓",
		StopColors:    []string{"fgGreen"},
	}

	spinner, err := yacspin.New(cfg)

	if err != nil {
		return nil, err
	}

	spinner.Message(fmt.Sprintf(source.GetName()))
	spinner.Start()
	tasks, err := source.GetTasks()
	spinner.Stop()
	if err != nil {
		return nil, err
	}

	for _, task := range tasks {
		if projectId != 0 && task.Project.TogglProject != projectId {
			continue
		}

		r := []*simpletable.Cell{
			{Align: simpletable.AlignLeft, Text: source.GetName()},
			{Align: simpletable.AlignLeft, Text: task.Key},
			{Align: simpletable.AlignLeft, Text: task.Summary},
			{Align: simpletable.AlignLeft, Text: task.Project.Key},
		}

		table.Body.Cells = append(table.Body.Cells, r)
	}

	return table, nil
}

// taskProjectFilter is the project to narrow a task listing to: none when
// --all is given, otherwise whichever project this repository maps to.
func taskProjectFilter(cmd *cobra.Command, cfg *workingon.Config) int {
	if all, _ := cmd.Flags().GetBool("all"); all {
		return 0
	}
	return workingon.FindProjectByGitRepositoryUrl(cfg)
}

func reportTasks(table *simpletable.Table, projectId int) {
	if len(table.Body.Cells) > 0 {
		fmt.Println(table.String())
		return
	}

	if projectId != 0 {
		fmt.Printf("No tasks found for project %d. Use --all to list the whole workspace.\n", projectId)
		return
	}
	fmt.Println("No tasks found.")
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

				projectId := taskProjectFilter(cmd, cfg)

				table, err := loadTasks(source, projectId)
				if err != nil {
					return err
				}

				reportTasks(table, projectId)
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

Narrowed to the toggl project this repository maps to, since that is almost
always the one you want. Use --all for the whole workspace.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			refresh, _ := cmd.Flags().GetBool("refresh")
			projectId := taskProjectFilter(cmd, cfg)

			for _, source := range workingon.Registry.RegisteredSources {
				if refresh {
					if err := refreshCache(source); err != nil {
						return err
					}
				}

				table, err := loadTasks(source, projectId)
				if err != nil {
					return err
				}

				reportTasks(table, projectId)
			}

			return nil
		},
	}
	tasksCommand.PersistentFlags().BoolP("refresh", "r", false,
		"Rebuild the local task cache before listing")
	tasksCommand.PersistentFlags().BoolP("all", "a", false,
		"List every task in the workspace, not just this project's")

	initConfigTasks(tasksCommand, cfg)
	return tasksCommand
}
