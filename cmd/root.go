package cmd

import (
	"fmt"
	"os"

	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
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

		// Errors are printed below instead, so that a run asking for JSON is
		// answered with JSON whether it went well or badly.
		SilenceErrors: true,
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
		NewModifyCommand(cfg),
		NewStopCommand(cfg),
		NewContinueCommand(cfg),
		NewCacheCommand(cfg),
		NewInitCommand(cfg),
		NewWhereCommand(cfg),
		NewSkillCommand(),
		NewVersionCommand(),
	)

	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false,
		"Print the result as JSON, for another program to read")

	rootCmd.Version = versionString()
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	if err := rootCmd.Execute(); err != nil {
		if jsonOutput {
			emitError(err)
		} else {
			fmt.Fprintln(os.Stderr, "Error:", err)
		}
		os.Exit(1)
	}
}

// configlessCommands run before there is anything to configure. `init` writes
// the config, and the rest only describe the binary - a shell asking what to
// complete, or someone asking what any of this does, should not be answered
// with a complaint about a file they have not written yet.
var configlessCommands = map[string]bool{
	"init":    true,
	"version": true,
	"help":    true,

	// `where` answers with what it found, and finding nothing is one of the
	// answers - refusing to say so because there is no config would leave the
	// one command that could explain the situation unable to run in it.
	"where": true,

	// `skill` writes a file out of the binary. Setting the agent up before
	// setting toggl up is a reasonable order to do things in.
	"skill": true,

	"completion":                    true,
	cobra.ShellCompRequestCmd:       true,
	cobra.ShellCompNoDescRequestCmd: true,
}

func needsConfig(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if configlessCommands[c.Name()] {
			return false
		}
	}
	return true
}
