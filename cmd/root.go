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
	// A missing config is not fatal here: `wo init` exists precisely to create
	// one, and it could never run if we bailed out first. Every other command
	// is stopped in PersistentPreRunE below.
	cfg, cfgErr := workingon.InitConfig()
	if cfgErr != nil {
		cfg = &workingon.Config{}
	} else {
		for _, source := range workingon.Registry.RegisteredSources {
			if err := source.Configure(cfg); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				os.Exit(1)
			}
		}
	}

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if cfgErr == nil || !needsConfig(cmd) {
			return nil
		}
		return fmt.Errorf("%s\n\nRun `wo init` to create one", cfgErr)
	}

	rootCmd.AddCommand(
		NewAddCommand(cfg),
		NewProjectsCommand(cfg),
		NewStartCommand(cfg),
		NewTasksCommand(cfg),
		NewTemplatesCommand(cfg),
		NewWhatCommand(cfg),
		NewNowCommand(cfg),
		NewShowCommand(cfg),
		NewSanitizeCommand(cfg),
		NewStopCommand(cfg),
		NewContinueCommand(cfg),
		NewCacheCommand(cfg),
		NewInitCommand(cfg),
		NewVersionCommand(),
	)

	rootCmd.Version = versionString()
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// configlessCommands run before there is anything to configure. `init` writes
// the config, and the rest only describe the binary - a shell asking what to
// complete, or someone asking what any of this does, should not be answered
// with a complaint about a file they have not written yet.
var configlessCommands = map[string]bool{
	"init":                          true,
	"version":                       true,
	"help":                          true,
	"completion":                    true,
	cobra.ShellCompRequestCmd:       true,
	cobra.ShellCompNoDescRequestCmd: true,
}

// needsConfig reports whether a command cannot run without a config file.
func needsConfig(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if configlessCommands[c.Name()] {
			return false
		}
	}
	return true
}
