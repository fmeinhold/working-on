package cmd

import (
	"github.com/spf13/cobra"
)

func newAddCommand() *cobra.Command {

	command := &cobra.Command{
		Use:   "add",
		Short: "Add a time entry",
		Long:  `Add a time entry`,

		RunE: func(cmd *cobra.Command, args []string) error {

			return nil
		},
	}

	return command

}
