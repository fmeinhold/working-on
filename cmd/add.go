package cmd

import (
	"fmt"
	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"strings"

	//"strconv"
	"time"
)

var (
	UnableToParseArgs = fmt.Errorf("unable to make sense of the arguments")
	DurationRequired  = fmt.Errorf("a duration is required")
	//NoProject         = fmt.Errorf("unable to figure out project")
)

type CommandArgs struct {
	Duration     time.Duration
	StartTime    time.Time
	SummaryOrKey string
}

type TaskDef struct {
	Name      string `mapstructure:"name"`
	TogglTask int64  `mapstructure:"toggl_task"`
	Start     string `mapstructure:"start"`
	Stop      string `mapstructure:"stop"`
	Alias     string `mapstructure:"alias"`
}

func NewAddCommand(cfg *workingon.Config) *cobra.Command {
	var (
		duration   time.Duration
		start      time.Time
		dryRun     bool
		tail       []string
		addCommand = &cobra.Command{
			Use:   "add <key|summary|template alias> <start time> <duration>",
			Short: "Add a time entry",
			Long: `Add a time entry

Either from a template set in your config file 
or by description/key, start time and duration`,

			Args: func(cmd *cobra.Command, args []string) error {
				var err error
				start, duration, tail, err = ParseArgs(newParseArgsConfig(cfg), args)
				return err
			},
			RunE: func(cmd *cobra.Command, args []string) error {

				templateArgs, err := cmd.Flags().GetStringToString("templateArgs")
				if err != nil {
					return err
				}

				wid, err := cmd.Flags().GetInt("wid")
				if err != nil {
					return err
				}

				project, err := cmd.Flags().GetString("project")
				if err != nil {
					return err
				}

				append_, err := cmd.Flags().GetBool("append")
				if err != nil {
					return err
				}

				if append_ {
					start, err = workingon.AppendStartTime(cfg)
					if err != nil {
						return err
					}
				}

				timeEntry, err := workingon.AddOrStart(cmd, cfg, wid, project, strings.Join(tail, " "), start,
					duration, templateArgs, false)
				if err != nil {
					return err
				}
				fmt.Println(timeEntry.Format(cfg.Settings.DateTimeLayout, &cfg.Settings.Location))

				return nil
			},
		}
	)

	// Flags
	addCommand.Flags().StringP("stop", "s", "", "Stop Time")
	addCommand.Flags().StringP("project", "p", viper.GetString("TOGGL_PROJECT"), "Set project")
	addCommand.Flags().BoolVarP(&dryRun, "dry", "d", false, "Do not create anything in toggl")
	addCommand.Flags().BoolP("append", "a", false, "Append to last time entry")
	addCommand.Flags().BoolP("fuzzy", "f", false, "Add some fuzziness to the start and stop time")
	addCommand.Flags().IntP("wid", "w", cfg.Settings.ToggleWid, "Toggle track workspace id")

	addCommand.Flags().StringToStringP("templateArgs", "t", nil, "List of named template args")

	return addCommand
}
