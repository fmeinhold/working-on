package workingon

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ErrProjectNotFound is returned when no source has a project by that name.
var ErrProjectNotFound = errors.New("project not found")

// ErrAmbiguousProject is returned when more than one project answers to the
// name. Booking against a guess would file the time under the wrong client, so
// the id is asked for instead.
var ErrAmbiguousProject = errors.New("more than one project has that name")

// FindProjectByName resolves a project name to its toggl id.
//
// Names are matched case insensitively, and an active project wins over an
// archived one of the same name - a name is reused after a project is closed
// far more often than someone means the closed one.
//
// Only projects that carry a toggl id can be resolved: a project belonging to
// some other source names no place an entry could be filed.
func FindProjectByName(name string) (int, error) {
	if name == "" {
		return 0, fmt.Errorf("%w: no name given", ErrProjectNotFound)
	}

	var active, archived []Project

	for _, source := range Registry.RegisteredSources {
		projects, err := source.GetProjects(true)
		if err != nil {
			return 0, fmt.Errorf("unable to list projects from %s: %w", source.GetName(), err)
		}

		for _, project := range projects {
			if !strings.EqualFold(project.Name, name) || projectId(project) == 0 {
				continue
			}
			if project.Archived {
				archived = append(archived, project)
			} else {
				active = append(active, project)
			}
		}
	}

	matches := active
	if len(matches) == 0 {
		matches = archived
	}

	switch len(matches) {
	case 0:
		return 0, fmt.Errorf("%w: no project named %q - `wo projects` lists them", ErrProjectNotFound, name)
	case 1:
		return projectId(matches[0]), nil
	default:
		return 0, fmt.Errorf("%w: %q is %s - name one by id instead",
			ErrAmbiguousProject, name, strings.Join(idsOf(matches), " or "))
	}
}

// projectId is the toggl id of a project, however the source records it. A
// toggl-native source sets TogglProject; the key is the id for any source that
// numbers its projects the same way.
func projectId(project Project) int {
	if project.TogglProject != 0 {
		return project.TogglProject
	}
	id, err := strconv.Atoi(project.Key)
	if err != nil {
		return 0
	}
	return id
}

// idsOf lists the ids of the projects that answered to an ambiguous name, in a
// stable order so the same collision reads the same way twice.
func idsOf(projects []Project) []string {
	ids := make([]string, 0, len(projects))
	for _, project := range projects {
		ids = append(ids, strconv.Itoa(projectId(project)))
	}
	sort.Strings(ids)
	return ids
}
