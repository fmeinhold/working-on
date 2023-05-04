package cmd

import (
	"fmt"
	"github.com/fmeinhold/workingon/cfg"
	"github.com/fmeinhold/workingon/toggl_api"
	"github.com/gosimple/slug"
	"github.com/peterh/liner"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/theckman/yacspin"
	"strings"
	"time"
)

func newInitCommand() *cobra.Command {
	command := &cobra.Command{
		Use: "init",

		RunE: func(cmd *cobra.Command, args []string) error {

			err := cfg.InitProjectConfig(true)
			if err != nil {
				return err
			}

			spinner, err := yacspin.New(yacspin.Config{
				Frequency:     100 * time.Millisecond,
				CharSet:       yacspin.CharSets[11],
				Suffix:        " Finished",
				StopCharacter: "✓",
				StopColors:    []string{"fgGreen"},
			})

			if err != nil {
				return err
			}

			client := toggl_api.NewToggl(cfg.GlobalConfig.GetString(cfg.TogglApiToken))

			spinner.Message(" Retrieving list of projects from toggl_api ... ")
			spinner.Start()

			projects, err := client.Projects.List(cfg.GlobalConfig.GetInt(cfg.TogglDefaultWid))
			spinner.Stop()

			fmt.Printf("%d projects retrieved from toggl_api. \n", len(projects))

			var defaultProject string
			var names []string

			for _, project := range projects {
				names = append(names, project.Name)
				fmt.Println(project.Name)
			}

			for {
				line := liner.NewLiner()
				defer line.Close()

				line.SetCtrlCAborts(true)

				line.SetCompleter(func(line string) (c []string) {
					for _, n := range names {
						if strings.Contains(strings.ToLower(n), strings.ToLower(line)) {
							c = append(c, n)
						}
					}
					return
				})

				projectName, err := line.Prompt("Enter toggl_api project name: ")
				if err != nil {
					return err
				}

				if len(strings.TrimSpace(projectName)) == 0 {
					break
				}

				line = liner.NewLiner()
				defer line.Close()

				projectSlug := slug.Make(projectName)

				toggleProject, err := line.PromptWithSuggestion("Enter toggl_api slug: ", projectSlug, len(projectSlug))
				if err != nil {
					return err
				}

				if defaultProject == "" {
					defaultProject = toggleProject
				}

				for _, project := range projects {
					if strings.ToLower(project.Name) == strings.ToLower(projectName) {
						cfg.ProjectConfig.Set(fmt.Sprintf("projects.%s.toggl_project_pid", toggleProject), project.Id)
						cfg.ProjectConfig.Set(fmt.Sprintf("projects.%s.toggl_project_name", toggleProject), project.Name)
					}
				}

			}

			line := liner.NewLiner()
			defer line.Close()

			defaultProject, err = line.PromptWithSuggestion("Enter default project name:", defaultProject, len(defaultProject))
			if err != nil {
				return err
			}

			cfg.ProjectConfig.Set("default_project", defaultProject)

			err = cfg.ProjectConfig.SafeWriteConfig()
			if err != nil {
				if _, ok := err.(viper.ConfigFileAlreadyExistsError); ok {
					err = cfg.ProjectConfig.WriteConfig()
					if err != nil {
						return err
					}
					fmt.Println("config written", cfg.ProjectConfig.ConfigFileUsed())
				}

				return err
			}

			return nil
		},
	}
	return command
}
