package cmd

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Overwritten at build time via -ldflags -X. The defaults are what a plain
// `go build` produces, where the module's VCS stamp is the only thing to go on.
var (
	version = ""
	commit  = ""
	date    = ""
)

func versionString() string {
	v, c, d := version, commit, date

	if v == "" || c == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			if v == "" && info.Main.Version != "" && info.Main.Version != "(devel)" {
				v = info.Main.Version
			}
			for _, setting := range info.Settings {
				switch setting.Key {
				case "vcs.revision":
					if c == "" {
						c = setting.Value
					}
				case "vcs.time":
					if d == "" {
						d = setting.Value
					}
				}
			}
		}
	}

	if v == "" {
		v = "dev"
	}
	if len(c) > 7 {
		c = c[:7]
	}

	s := v
	if c != "" {
		s += fmt.Sprintf(" (%s)", c)
	}
	if d != "" {
		s += fmt.Sprintf(" built %s", d)
	}
	return fmt.Sprintf("%s %s/%s %s", s, runtime.GOOS, runtime.GOARCH, runtime.Version())
}

func NewVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of wo",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), versionString())
		},
	}
}
