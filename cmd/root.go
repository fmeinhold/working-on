package cmd

import (
	"github.com/spf13/cobra"
	"os"
)

var (
	rootCmd = &cobra.Command{
		Use:   "wo",
		Short: "Working on helps you track what you're working on.",
		Long: `                     __   .__                                
__  _  _____________|  | _|__| ____    ____     ____   ____  
\ \/ \/ /  _ \_  __ \  |/ /  |/    \  / ___\   /  _ \ /    \ 
 \     (  <_> )  | \/    <|  |   |  \/ /_/  > (  <_> )   |  \
  \/\_/ \____/|__|  |__|_ \__|___|  /\___  /   \____/|___|  /
                         \/       \//_____/               \/ 

`,
		SilenceUsage: true,
	}
)

func Execute() {
	rootCmd.AddCommand(
		newInitCommand(),
		newStartCommand(),
		newSetupCommand(),
		newListCommand(),
		newNowCommand(),
		newStopCommand(),
		newContinueCommand(),
		newWorkspacesCommand(),
	)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
