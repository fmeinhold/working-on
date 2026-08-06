package cmd

import (
	"fmt"
	"github.com/fefeme/workingon/workingon"
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
	cfg, err := workingon.InitConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	for _, source := range workingon.Registry.RegisteredSources {
		if err := source.Configure(cfg); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	}
	rootCmd.AddCommand(
		NewAddCommand(cfg),
		NewProjectsCommand(cfg),
		NewStartCommand(cfg),
		NewTasksCommand(cfg),
		NewWhatCommand(cfg),
		NewStopCommand(cfg),
		NewContinueCommand(cfg),
		NewCacheCommand(cfg),
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
