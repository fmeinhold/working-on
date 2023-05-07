package cmd

import (
	"fmt"
	"github.com/fmeinhold/workingon/cfg"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"os"
)

var (
	Pid int
	Wid int
)

func bindFlag(name string, flags *pflag.FlagSet) error {

	f := flags.Lookup(name)

	if !f.Changed && cfg.GlobalConfig.IsSet(cfg.TogglDefaultPid) {
		val := cfg.GlobalConfig.Get(cfg.TogglDefaultPid)
		err := flags.Set(f.Name, fmt.Sprintf("%v", val))
		if err != nil {
			return err
		}
	}

	return nil
}

func newRootCommand() (*cobra.Command, error) {
	rootCmd := &cobra.Command{
		Use:   "wo",
		Short: "Working on helps you track what you're working on.",
		Long: `                     __   .__                                
__  _  _____________|  | _|__| ____    ____     ____   ____  
\ \/ \/ /  _ \_  __ \  |/ /  |/    \  / ___\   /  _ \ /    \ 
 \     (  <_> )  | \/    <|  |   |  \/ /_/  > (  <_> )   |  \
  \/\_/ \____/|__|  |__|_ \__|___|  /\___  /   \____/|___|  /
                         \/       \//_____/               \/ 

`,
		SilenceUsage: false,
	}

	err := cfg.InitGlobalConfig()
	if err != nil {
		return nil, err
	}

	rootCmd.PersistentFlags().IntVarP(&Pid, "pid", "p", -1, "Set toggl project id")
	rootCmd.PersistentFlags().IntVarP(&Wid, "wid", "w", -1, "Set toggl workspace id")

	err = bindFlag("pid", rootCmd.PersistentFlags())
	if err != nil {
		return nil, err
	}

	err = bindFlag("wid", rootCmd.PersistentFlags())
	if err != nil {
		return nil, err
	}

	return rootCmd, nil
}

func Execute() {
	rootCmd, err := newRootCommand()
	if err != nil {
		panic(err)
	}
	rootCmd.AddCommand(
		newAddCommand(),
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
		fmt.Println(err)
		os.Exit(1)
	}
}
