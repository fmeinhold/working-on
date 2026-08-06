package toggl

import (
	"fmt"
	"net/url"
	"strconv"
)

// projectsPerPage is the page size used when walking the workspace project
// list. The endpoint defaults to 151 and answers sorted by name, so anything
// past that is dropped without a word - and what survives looks arbitrary
// rather than obviously truncated.
const projectsPerPage = 1000

type WorkspaceList struct {
	Count      int
	Workspaces []Workspace
}

type ProjectList struct {
	Count    int
	Projects []Project
}

type WorkspaceClient struct {
	client *Client
}

func (w *WorkspaceClient) GetWorkspaces() (*WorkspaceList, error) {
	message, err := w.client.NewMessage("GET", "me/workspaces", nil)
	if err != nil {
		return nil, err
	}

	data, err := w.client.SendRequest(message)
	if err != nil {
		return nil, err
	}

	var workspaces []Workspace
	if err := decodeList(*data, &workspaces); err != nil {
		return nil, err
	}

	return &WorkspaceList{
		Workspaces: workspaces,
		Count:      len(workspaces),
	}, nil
}

// ProjectQuery narrows a project listing.
type ProjectQuery struct {
	// ActiveOnly leaves out archived projects. In a workspace with any history
	// they outnumber the live ones several times over, so a listing meant for
	// a person to read wants them gone.
	ActiveOnly bool
}

// ListProjects returns every project in the workspace, archived ones included.
func (w *WorkspaceClient) ListProjects(wid int) (*ProjectList, error) {
	return w.ListProjectsWhere(wid, ProjectQuery{})
}

// ListProjectsWhere returns the projects matching query, following pagination
// to the end.
func (w *WorkspaceClient) ListProjectsWhere(wid int, query ProjectQuery) (*ProjectList, error) {
	var projects []Project

	for page := 1; ; page++ {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("per_page", strconv.Itoa(projectsPerPage))
		if query.ActiveOnly {
			params.Set("active", "true")
		}

		message, err := w.client.NewMessage("GET",
			fmt.Sprintf("workspaces/%d/projects?%s", wid, params.Encode()), nil)
		if err != nil {
			return nil, err
		}

		data, err := w.client.SendRequest(message)
		if err != nil {
			return nil, err
		}

		var rows []Project
		if err := decodeList(*data, &rows); err != nil {
			return nil, err
		}

		projects = append(projects, rows...)

		// Unlike the task listing this endpoint answers with a bare array and
		// no total, so a short page is the only end marker there is.
		if len(rows) < projectsPerPage {
			break
		}
	}

	return &ProjectList{
		Projects: projects,
		Count:    len(projects),
	}, nil
}
