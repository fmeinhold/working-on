package cmd

import (
	"testing"

	"github.com/fefeme/workingon/workingon"

	"github.com/spf13/cobra"
)

// tasksCommandWith builds the command as NewTasksCommand does, so the flags the
// filter reads are the real ones, and applies the given arguments.
func tasksCommandWith(t *testing.T, cfg *workingon.Config, args ...string) *cobra.Command {
	t.Helper()

	command := NewTasksCommand(cfg)
	if err := command.ParseFlags(args); err != nil {
		t.Fatal(err)
	}

	return command
}

// A .workingon.yaml names the project for a checkout by setting the default,
// so the default is what "this project" means.
func TestTaskProjectFilterUsesTheConfiguredDefault(t *testing.T) {
	cfg := &workingon.Config{}
	cfg.Settings.ToggleDefaultPid = 91210706

	if got := taskProjectFilter(tasksCommandWith(t, cfg), cfg); got.projectId != 91210706 {
		t.Errorf("filter = %+v, want the default project", got)
	}
}

// With no default anywhere there is no project to narrow to, and the listing
// must not silently become the whole workspace pretending to be one.
func TestTaskProjectFilterIsInactiveWithoutADefault(t *testing.T) {
	cfg := &workingon.Config{}

	if got := taskProjectFilter(tasksCommandWith(t, cfg), cfg); got.active() {
		t.Errorf("filter = %+v, want no project", got)
	}
}

func TestTaskProjectFilterListsEverythingWithAll(t *testing.T) {
	cfg := &workingon.Config{}
	cfg.Settings.ToggleDefaultPid = 91210706

	if got := taskProjectFilter(tasksCommandWith(t, cfg, "--all"), cfg); got.active() {
		t.Errorf("filter = %+v, want no filter at all", got)
	}
}

// The subcommands take the flag from the parent, and read it the same way.
func TestTaskProjectFilterAllReachesTheSourceSubcommands(t *testing.T) {
	cfg := &workingon.Config{}
	cfg.Settings.ToggleDefaultPid = 91210706

	command := NewTasksCommand(cfg)

	sub, _, err := command.Find([]string{"toggl"})
	if err != nil || sub == command {
		t.Skip("no source subcommand is registered in this test binary")
	}

	if err := sub.ParseFlags([]string{"--all"}); err != nil {
		t.Fatal(err)
	}

	if got := taskProjectFilter(sub, cfg); got.active() {
		t.Errorf("filter = %+v, want no filter at all", got)
	}
}
