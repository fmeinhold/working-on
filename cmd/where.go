package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewWhereCommand(cfg *workingon.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "where",
		Short: "Show which config applies here, and where time booked here goes",
		Long: `Show which config applies in this directory.

Time is filed against the checkout you are standing in, so the answer depends on
where you ask from. This says which files were read, and which project an entry
started here would land in.

A checkout with no ` + "`.workingon.yaml`" + ` of its own is not set up for anything in
particular - entries from it fall back to your global default project, if you
have one. Run ` + "`wo init`" + ` there to give it one.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			where := whereWeAre(cfg)

			if jsonOutput {
				return emit(where)
			}

			fmt.Print(renderWhere(where))
			return nil
		},
	}
}

// Configured is the question worth asking first and is spelled out rather than
// left to be inferred from LocalConfig being null - a caller deciding whether
// this checkout is one that books time should not have to know that the two
// mean the same thing.
type whereJSON struct {
	Directory    string `json:"directory"`
	LocalConfig  string `json:"local_config,omitempty"`
	GlobalConfig string `json:"global_config,omitempty"`
	Project      *ref   `json:"project"`
	Configured   bool   `json:"configured"`
}

func whereWeAre(cfg *workingon.Config) whereJSON {
	directory, err := os.Getwd()
	if err != nil {
		directory = ""
	}

	local := workingon.FindLocalConfig()

	where := whereJSON{
		Directory:    directory,
		LocalConfig:  local,
		GlobalConfig: viper.ConfigFileUsed(),
		Configured:   local != "",
	}

	if project := workingon.CurrentProject(cfg); project != 0 {
		where.Project = named(project, lookupProjectName(project))
	}

	return where
}

func renderWhere(where whereJSON) string {
	out := fmt.Sprintf("Directory      %s\n", where.Directory)

	out += "Repository     "
	if where.LocalConfig == "" {
		out += "no .workingon.yaml - `wo init` here to set one up\n"
	} else {
		out += filepath.Dir(where.LocalConfig) + "\n"
	}

	if where.GlobalConfig != "" {
		out += fmt.Sprintf("Config         %s\n", where.GlobalConfig)
	}

	out += "Project        "
	switch {
	case where.Project == nil:
		out += "none - no toggl_default_pid is set\n"
	case where.Project.Name == "":
		out += fmt.Sprintf("%d\n", where.Project.Id)
	default:
		out += fmt.Sprintf("%s (%d)\n", where.Project.Name, where.Project.Id)
	}

	return out
}
