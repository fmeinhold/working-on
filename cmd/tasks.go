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

func loadTasks(source workingon.Source) (*simpletable.Table, error) {
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

func initConfigTasks(tasksCommand *cobra.Command) {
	for i, _ := range workingon.Registry.RegisteredSources {
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
				table, err := loadTasks(source)
				if err != nil {
					return err
				}
				if (len(table.Body.Cells)) > 0 {
					fmt.Println(table.String())
				} else {
					fmt.Println("No tasks found.")
				}
				return nil

			},
		})
	}
}

func NewTasksCommand(cfg *workingon.Config) *cobra.Command {
	var tasksCommand = &cobra.Command{
		Use:   "tasks",
		Short: "List all tasks from all sources",
		Long:  `List all tasks from all sources`,
		RunE: func(cmd *cobra.Command, args []string) error {
			refresh, _ := cmd.Flags().GetBool("refresh")

			for _, source := range workingon.Registry.RegisteredSources {
				if refresh {
					if err := refreshCache(source); err != nil {
						return err
					}
				}
				table, err := loadTasks(source)
				if err != nil {
					return err
				}
				if (len(table.Body.Cells)) > 0 {
					fmt.Println(table.String())
				} else {
					fmt.Println("No tasks found.")
				}

			}

			return nil
		},
	}
	tasksCommand.PersistentFlags().BoolP("refresh", "r", false,
		"Rebuild the local task cache before listing")

	initConfigTasks(tasksCommand)
	return tasksCommand
}
