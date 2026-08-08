package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// The commands that describe the binary have to run before there is a config,
// or `wo init` is unreachable and a shell asking what to complete - which is
// what homebrew does while installing - gets an error instead of a script.
func TestNeedsConfig(t *testing.T) {
	root := &cobra.Command{Use: "wo"}

	child := func(parent *cobra.Command, name string) *cobra.Command {
		cmd := &cobra.Command{Use: name}
		parent.AddCommand(cmd)
		return cmd
	}

	completion := child(root, "completion")
	fish := child(completion, "fish")

	for _, tc := range []struct {
		cmd  *cobra.Command
		want bool
	}{
		{child(root, "init"), false},
		{child(root, "version"), false},
		{child(root, "help"), false},
		{child(root, "where"), false},
		{completion, false},

		// A shell asks the subcommand, not the parent.
		{fish, false},

		// What the generated scripts actually call.
		{child(root, cobra.ShellCompRequestCmd), false},
		{child(root, cobra.ShellCompNoDescRequestCmd), false},

		// Everything that talks to toggl still needs one.
		{child(root, "start"), true},
		{child(root, "show"), true},
	} {
		if got := needsConfig(tc.cmd); got != tc.want {
			t.Errorf("needsConfig(%q) = %v, want %v", tc.cmd.Name(), got, tc.want)
		}
	}
}
