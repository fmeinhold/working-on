package cmd

import (
	"fmt"
	"github.com/fefeme/workingon/util"
	"github.com/fefeme/workingon/workingon"
	"time"

	"github.com/alexeyco/simpletable"
	"github.com/spf13/cobra"
	"github.com/theckman/yacspin"
)

func NewProjectsCommand(cfg *workingon.Config) *cobra.Command {
	var projectsCommand = &cobra.Command{
		Use:   "projects",
		Short: "List all projects",
		Long:  `List all projects`,
		RunE: func(cmd *cobra.Command, args []string) error {

			cfg := yacspin.Config{
				Frequency:     100 * time.Millisecond,
				CharSet:       yacspin.CharSets[11],
				Suffix:        " retrieving projects ...",
				StopCharacter: "✓",
				StopColors:    []string{"fgGreen"},
			}

			spinner, err := yacspin.New(cfg)

			if err != nil {
				panic(err)
			}

			table := simpletable.New()

			table.Header = &simpletable.Header{
				Cells: []*simpletable.Cell{
					{Align: simpletable.AlignLeft, Text: "Key"},
					{Align: simpletable.AlignLeft, Text: "Name"},
				},
			}

			for _, source := range workingon.Registry.RegisteredSources {
				if len(args) > 0 && !util.StringInSliceI(source.GetName(), args) {
					continue
				}
				spinner.Message(fmt.Sprintf(source.GetName()))
				spinner.Start()
				projects, err := source.GetProjects()
				spinner.Stop()

				if len(projects) > 0 {
					fmt.Println(source.GetName())
				}

				if err != nil {
					return err
				}

				for _, project := range projects {
					r := []*simpletable.Cell{
						{Align: simpletable.AlignLeft, Text: project.Key},
						{Align: simpletable.AlignLeft, Text: project.Name},
					}

					table.Body.Cells = append(table.Body.Cells, r)
				}
				table.SetStyle(simpletable.StyleCompactLite)
				fmt.Println(table.String())
			}
			return nil

		},
	}
	return projectsCommand
}
