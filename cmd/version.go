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

// buildJSON is what this binary is, in parts - the same facts versionString
// runs together into a line.
type buildJSON struct {
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
	Date    string `json:"date,omitempty"`
	Os      string `json:"os"`
	Arch    string `json:"arch"`
	Go      string `json:"go"`
}

func buildInfo() buildJSON {
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

	return buildJSON{
		Version: v,
		Commit:  c,
		Date:    d,
		Os:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		Go:      runtime.Version(),
	}
}

func versionString() string {
	info := buildInfo()

	s := info.Version
	if info.Commit != "" {
		s += fmt.Sprintf(" (%s)", info.Commit)
	}
	if info.Date != "" {
		s += fmt.Sprintf(" built %s", info.Date)
	}

	return fmt.Sprintf("%s %s/%s %s", s, info.Os, info.Arch, info.Go)
}

func NewVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of wo",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOutput {
				return emit(buildInfo())
			}

			fmt.Fprintln(cmd.OutOrStdout(), versionString())
			return nil
		},
	}
}
