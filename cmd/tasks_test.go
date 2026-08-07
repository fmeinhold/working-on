package cmd

import (
	"testing"

	"github.com/fefeme/workingon/workingon"

	"github.com/spf13/cobra"
	"github.com/tcnksm/go-gitconfig"
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

// A .workingon.yaml names the project for a checkout that no mapping matches,
// and that is as much "this project" as a mapping is.
func TestTaskProjectFilterUsesTheConfiguredDefault(t *testing.T) {
	cfg := &workingon.Config{}
	cfg.Settings.ToggleDefaultPid = 91210706

	if got := taskProjectFilter(tasksCommandWith(t, cfg), cfg); got.projectId != 91210706 {
		t.Errorf("filter = %+v, want the default project", got)
	}
}

func TestTaskProjectFilterPrefersTheRepositoryMapping(t *testing.T) {
	origin, err := gitconfig.OriginURL()
	if err != nil || origin == "" {
		t.Skip("not in a checkout with an origin to match against")
	}

	cfg := &workingon.Config{
		Projects: []workingon.ProjectMapping{{Name: "here", TogglePid: 188362780, Git: origin}},
	}
	cfg.Settings.ToggleDefaultPid = 91210706

	if got := taskProjectFilter(tasksCommandWith(t, cfg), cfg); got.projectId != 188362780 {
		t.Errorf("filter = %+v, want the mapped project", got)
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
