package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
)

// cacheKeeper is the optional capability of a source that maintains a local
// cache. Sources without one are simply skipped, so the Source interface does
// not have to carry caching it may not do.
type cacheKeeper interface {
	RefreshCache() error
	ClearCache() error
}

// cacheLocator is the further optional ability to say where that cache lives.
type cacheLocator interface {
	CachePath() string
}

func cacheKeepers() []workingon.Source {
	var keepers []workingon.Source
	for _, source := range workingon.Registry.RegisteredSources {
		if _, ok := source.(cacheKeeper); ok {
			keepers = append(keepers, source)
		}
	}
	return keepers
}

func refreshCache(source workingon.Source) error {
	keeper, ok := source.(cacheKeeper)
	if !ok {
		return nil
	}
	return keeper.RefreshCache()
}

func NewCacheCommand(cfg *workingon.Config) *cobra.Command {
	cacheCommand := &cobra.Command{
		Use:   "cache",
		Short: "Manage the local task cache",
		Long: `Manage the local task cache.

Toggl has no lookup for a task by id alone, so resolving one means walking the
whole workspace task list. Working On keeps a local copy and tops it up with
small delta requests instead.`,
	}

	cacheCommand.AddCommand(newCacheClearCommand(), newCacheStatusCommand())

	return cacheCommand
}

func newCacheClearCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Delete the local task cache",
		Long: `Delete the local task cache.

The next command that needs a task rebuilds it. Clearing is never required in
normal use - a stale cache refreshes itself - but it is here for when you want
to be certain.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			keepers := cacheKeepers()
			if len(keepers) == 0 {
				fmt.Println("No source keeps a local cache.")
				return nil
			}

			var cleared []string
			for _, source := range keepers {
				if err := source.(cacheKeeper).ClearCache(); err != nil {
					return fmt.Errorf("unable to clear the %s cache: %w", source.GetName(), err)
				}
				cleared = append(cleared, source.GetName())
			}

			fmt.Printf("Cleared the task cache for: %s\n", strings.Join(cleared, ", "))
			return nil
		},
	}
}

func newCacheStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show where the task cache lives and whether it exists",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			keepers := cacheKeepers()
			if len(keepers) == 0 {
				fmt.Println("No source keeps a local cache.")
				return nil
			}

			for _, source := range keepers {
				locator, ok := source.(cacheLocator)
				if !ok {
					fmt.Printf("%s: caching enabled\n", source.GetName())
					continue
				}

				path := locator.CachePath()
				if path == "" {
					fmt.Printf("%s: caching disabled, no writable cache directory\n", source.GetName())
					continue
				}

				info, err := os.Stat(path)
				if err != nil {
					fmt.Printf("%s: no cache yet (%s)\n", source.GetName(), path)
					continue
				}

				fmt.Printf("%s: %s (%.1f KiB, updated %s)\n", source.GetName(), path,
					float64(info.Size())/1024, info.ModTime().Format("2006-01-02 15:04:05"))
			}

			return nil
		},
	}
}
