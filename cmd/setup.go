package cmd

import (
	"fmt"
	"github.com/fmeinhold/workingon/cfg"
	"github.com/fmeinhold/workingon/toggl_api"
	"github.com/peterh/liner"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func PromptConfig(config *viper.Viper, prompt string, key string, suggestion string) (string, bool, error) {
	if suggestion == "" {
		suggestion = config.GetString(key)
	}

	line := liner.NewLiner()
	line.SetCtrlCAborts(true)

	defer line.Close()

	result, err := line.PromptWithSuggestion(prompt, suggestion, -1)
	if err != nil {
		return "", true, err
	}

	cfg.GlobalConfig.Set(key, result)

	return result, false, nil
}

func newSetupCommand() *cobra.Command {
	return &cobra.Command{
		Use: "setup",

		RunE: func(cmd *cobra.Command, args []string) error {
			_, skipped, _ := PromptConfig(cfg.GlobalConfig, "Enter toggl_api track api token: ", "toggl_api.api_token", "")
			if skipped {
				fmt.Println("Configuration cancelled.")
				return nil
			}

			toggl := toggl_api.NewToggl(cfg.GlobalConfig.GetString(cfg.TogglApiToken))
			me, err := toggl.Me.Get()
			if err != nil {
				return err
			}

			_, skipped, _ = PromptConfig(cfg.GlobalConfig, "Enter toggl_api default workspace id: ", cfg.TogglDefaultWid, fmt.Sprintf("%d", me.DefaultWorkspaceId))

			_, skipped, _ = PromptConfig(cfg.GlobalConfig, "Enter jira username (leave empty to skip): ", "jira.username", "")

			fmt.Println("Get a jira token from https://id.atlassian.com/manage-profile/security/api-tokens")

			_, skipped, _ = PromptConfig(cfg.GlobalConfig, "Enter jira api token (leave empty to skip): ", "jira.api_token", "")

			err = cfg.GlobalConfig.SafeWriteConfig()
			if err != nil {
				return err
			}

			return nil
		},
	}
}
