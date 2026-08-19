package workingon

import (
	"fmt"
	"sort"
	"time"

	"github.com/fefeme/workingon/toggl"
)

// RecentDays is how far back the recent listing looks.
//
// Asked for no range at all toggl answers with the last few days, which is
// thin after a week away - and the whole point of this listing is picking up
// something you were doing before the break.
const RecentDays = 30

// RecentEntries are the entries worth continuing, most recent first.
//
// The same work booked five times over is one thing to pick up, not five, so
// entries are folded together on what a continuation would copy - description,
// project and task - keeping the most recent of each. A timer still running is
// left out: there is nothing to continue about work that has not stopped.
func RecentEntries(cfg *Config) ([]toggl.TimeEntry, error) {
	return recentEntries(toggl.NewToggl(cfg.Settings.ToggleApiToken), time.Now())
}

func recentEntries(client *toggl.Toggl, now time.Time) ([]toggl.TimeEntry, error) {
	from := now.AddDate(0, 0, -RecentDays)
	to := now.AddDate(0, 0, 1)

	listed, err := client.TimeEntries.List(&from, &to)
	if err != nil {
		return nil, err
	}

	entries := listed.TimeEntries

	// v9 does not document the order it answers in, so it is sorted here
	// rather than trusted - which of two identical entries is kept depends on
	// it entirely.
	sort.SliceStable(entries, func(i, j int) bool {
		left, right := entries[i].Start, entries[j].Start
		if left == nil || right == nil {
			return right == nil && left != nil
		}
		return left.After(*right)
	})

	seen := make(map[string]bool, len(entries))
	recent := make([]toggl.TimeEntry, 0, len(entries))

	for i := range entries {
		entry := entries[i]

		if entry.Start == nil || entry.IsRunning() {
			continue
		}

		key := fmt.Sprintf("%s\x00%d\x00%d", entry.Description, entry.ProjectId, entry.TaskId)
		if seen[key] {
			continue
		}

		seen[key] = true
		recent = append(recent, entry)
	}

	return recent, nil
}

// ContinueEntry opens a fresh timer carrying one particular entry's
// description, project and task, rather than whichever was last.
//
// The entry it copies keeps its own record, the same as continuing the last
// one does - this starts something new that looks like it.
func ContinueEntry(cfg *Config, previous *toggl.TimeEntry, req EntryRequest) (*toggl.TimeEntry, error) {
	if previous == nil {
		return nil, fmt.Errorf("there is no entry to continue")
	}
	if previous.IsRunning() {
		return nil, fmt.Errorf("%q is already running", previous.Description)
	}

	client := toggl.NewToggl(cfg.Settings.ToggleApiToken)

	return finishContinuation(client, cfg, copyOf(previous), req)
}
