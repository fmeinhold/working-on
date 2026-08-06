package workingon

import (
	"errors"
	"fmt"
	"strings"
)

// ErrTaskNotFound is returned by a source that answered correctly but has no
// such task. It is deliberately distinct from a source that could not answer
// at all, so an unreachable source is never reported as a missing task.
var ErrTaskNotFound = errors.New("task not found")

// ErrNoSourceClaimsKey is returned when no configured source recognises the
// key. That is how a plain description is told apart from a task reference
// that failed to resolve, so callers can fall back to free text without
// swallowing a genuine lookup failure.
var ErrNoSourceClaimsKey = errors.New("not a task key for any configured source")

type Source interface {
	Configure(config *Config) error

	GetName() string

	// Handles reports whether key looks like a task key this source owns.
	// The registry skips sources that answer false, so a key one source owns
	// never costs a round trip to another.
	Handles(key string) bool

	GetTask(key string) (*Task, error)
	GetTasks() ([]Task, error)

	// GetProjects lists the source's projects. Archived ones are left out
	// unless asked for: they pile up without limit and are not what someone
	// listing projects is looking for.
	GetProjects(includeArchived bool) ([]Project, error)
}

type registry struct {
	RegisteredSources []Source
}

var (
	Registry registry
)

func (r *registry) Register(source Source) error {
	r.RegisteredSources = append(r.RegisteredSources, source)
	return nil
}

func (r *registry) GetNames() []string {
	var names []string
	for _, source := range r.RegisteredSources {
		names = append(names, source.GetName())
	}
	return names
}

// GetTask resolves a task key against the sources that claim it.
//
// A key no source claims - free text, most commonly - resolves to an error
// without any network call at all, which is what callers rely on to tell a
// task key from a plain description.
func (r *registry) GetTask(key string) (*Task, error) {
	var (
		asked    []string
		failures []string
	)

	for _, source := range r.RegisteredSources {
		if !source.Handles(key) {
			continue
		}

		asked = append(asked, source.GetName())

		task, err := source.GetTask(key)
		switch {
		case errors.Is(err, ErrTaskNotFound):
			// A clean "no such task"; keep looking, but do not treat it as a
			// failure of the source.
		case err != nil:
			failures = append(failures, fmt.Sprintf("%s: %v", source.GetName(), err))
		case task != nil:
			return task, nil
		}
	}

	switch {
	case len(asked) == 0:
		return nil, fmt.Errorf("%w: %q", ErrNoSourceClaimsKey, key)
	case len(failures) > 0:
		// Say why rather than claiming it does not exist - the source may
		// simply be unreachable.
		return nil, fmt.Errorf("unable to look up task %q (%s)", key, strings.Join(failures, "; "))
	default:
		return nil, fmt.Errorf("task %q not found in %s", key, strings.Join(asked, ", "))
	}
}
