package toggl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// pagedTaskServer serves total tasks with ids 1..total through the paginated
// envelope the workspace task endpoint really uses, honouring page/per_page.
// It records the pages actually requested.
func pagedTaskServer(t *testing.T, total int) (*TaskClient, *[]int) {
	t.Helper()

	var pages []int

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		if page == 0 {
			page = 1
		}
		if perPage == 0 {
			perPage = 50
		}
		pages = append(pages, page)

		first := (page - 1) * perPage
		last := first + perPage
		if last > total {
			last = total
		}

		var rows []string
		for i := first; i < last; i++ {
			rows = append(rows, fmt.Sprintf(
				`{"id":%d,"name":"task %d","project_id":%d,"workspace_id":5}`, i+1, i+1, 100+i%3))
		}

		fmt.Fprintf(w, `{"data":[%s],"page":%d,"per_page":%d,"total_count":%d}`,
			strings.Join(rows, ","), page, perPage, total)
	})

	return &TaskClient{client: client}, &pages
}

// The endpoint defaults to 50 rows per page. Without pagination a real
// workspace is silently truncated - the live account this was built against
// has 7512 tasks.
func TestListFollowsPaginationToTheEnd(t *testing.T) {
	const total = 2500

	tasks, pages := pagedTaskServer(t, total)

	list, err := tasks.List(5)
	if err != nil {
		t.Fatal(err)
	}

	if list.Count != total {
		t.Fatalf("got %d tasks, want %d", list.Count, total)
	}
	if len(*pages) < 2 {
		t.Errorf("fetched %d page(s); expected the walk to paginate", len(*pages))
	}
	if list.Tasks[0].Id != 1 || list.Tasks[total-1].Id != total {
		t.Errorf("got ids %d..%d, want 1..%d", list.Tasks[0].Id, list.Tasks[total-1].Id, total)
	}
}

func TestListHandlesExactPageBoundary(t *testing.T) {
	tasks, _ := pagedTaskServer(t, tasksPerPage)

	list, err := tasks.List(5)
	if err != nil {
		t.Fatal(err)
	}
	if list.Count != tasksPerPage {
		t.Errorf("got %d tasks, want %d", list.Count, tasksPerPage)
	}
}

func TestListAcceptsBareArrayResponse(t *testing.T) {
	client := newTestClient(t, respondWith(http.StatusOK,
		`[{"id":1,"name":"only","project_id":91,"workspace_id":5}]`))
	tasks := &TaskClient{client: client}

	list, err := tasks.List(5)
	if err != nil {
		t.Fatal(err)
	}
	if list.Count != 1 || list.Tasks[0].Name != "only" {
		t.Errorf("got %+v, want a single task named \"only\"", list.Tasks)
	}
}

// Find has to walk the listing because v9 dropped the id-only task route, but
// it should stop as soon as it has a match rather than reading everything.
func TestFindStopsAtTheMatchingPage(t *testing.T) {
	tasks, pages := pagedTaskServer(t, 10*tasksPerPage)

	found, err := tasks.Find(5, 3)
	if err != nil {
		t.Fatal(err)
	}
	if found.Id != 3 {
		t.Errorf("found task %d, want 3", found.Id)
	}
	if len(*pages) != 1 {
		t.Errorf("read %d pages for a first-page hit, want 1", len(*pages))
	}
}

func TestFindReachesTasksBeyondTheFirstPage(t *testing.T) {
	const total = 3 * tasksPerPage

	tasks, pages := pagedTaskServer(t, total)

	found, err := tasks.Find(5, total)
	if err != nil {
		t.Fatal(err)
	}
	if found.Id != total {
		t.Errorf("found task %d, want %d", found.Id, total)
	}
	if len(*pages) < 3 {
		t.Errorf("read %d pages; the match is on page 3", len(*pages))
	}
}

func TestFindReportsMissingTask(t *testing.T) {
	tasks, _ := pagedTaskServer(t, 10)

	_, err := tasks.Find(5, 999)
	if err == nil {
		t.Fatal("expected an error for an unknown task id")
	}
	if !strings.Contains(err.Error(), "999") {
		t.Errorf("error %q should name the missing task id", err)
	}
}

// v9 addresses a single task through its project, unlike v8's /tasks/{id}.
func TestGetUsesProjectScopedPath(t *testing.T) {
	var path string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		fmt.Fprint(w, `{"id":42,"name":"standup","project_id":91,"workspace_id":5}`)
	})

	got, err := (&TaskClient{client: client}).Get(5, 91, 42)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/workspaces/5/projects/91/tasks/42" {
		t.Errorf("path = %s, want /workspaces/5/projects/91/tasks/42", path)
	}
	if got.Name != "standup" || got.ProjectId != 91 {
		t.Errorf("got %+v, want standup on project 91", got)
	}
}

func TestListProjectsAndWorkspaces(t *testing.T) {
	var path string

	projectClient := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		fmt.Fprint(w, `[{"id":91,"name":"SW","workspace_id":5,"active":true,"color":"#06a893"}]`)
	})

	projects, err := (&WorkspaceClient{client: projectClient}).ListProjects(5)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/workspaces/5/projects" {
		t.Errorf("path = %s, want /workspaces/5/projects", path)
	}
	if projects.Projects[0].WorkspaceId != 5 || projects.Projects[0].Name != "SW" {
		t.Errorf("got %+v, want SW in workspace 5", projects.Projects[0])
	}

	workspaceClient := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		fmt.Fprint(w, `[{"id":5,"name":"Sealworks","organization_id":9}]`)
	})

	workspaces, err := (&WorkspaceClient{client: workspaceClient}).GetWorkspaces()
	if err != nil {
		t.Fatal(err)
	}
	if path != "/me/workspaces" {
		t.Errorf("path = %s, want /me/workspaces", path)
	}
	if workspaces.Workspaces[0].OrganizationId != 9 {
		t.Errorf("organization id = %d, want 9", workspaces.Workspaces[0].OrganizationId)
	}
}

// The v9 field names must survive a round trip; v8's short names are gone.
func TestTaskDecodesV9FieldNames(t *testing.T) {
	var task Task

	body := `{"id":1,"name":"n","project_id":91,"project_name":"SW","workspace_id":5,
	          "user_id":7,"estimated_seconds":60,"tracked_seconds":30,"active":true}`

	if err := json.Unmarshal([]byte(body), &task); err != nil {
		t.Fatal(err)
	}

	for field, got := range map[string]int{
		"project_id":        task.ProjectId,
		"workspace_id":      task.WorkspaceId,
		"user_id":           task.UserId,
		"estimated_seconds": task.EstimatedSeconds,
		"tracked_seconds":   task.TrackedSeconds,
	} {
		if got == 0 {
			t.Errorf("%s did not decode", field)
		}
	}
	if task.ProjectName != "SW" {
		t.Errorf("project_name = %q, want SW", task.ProjectName)
	}
}

// The endpoint defaults to a page of 151 sorted by name, so a workspace with
// more projects than that loses the tail without any indication.
func TestListProjectsFollowsPagination(t *testing.T) {
	var pages []string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		pages = append(pages, r.URL.Query().Get("page"))

		var rows []string
		if r.URL.Query().Get("page") == "1" {
			for i := 0; i < projectsPerPage; i++ {
				rows = append(rows, fmt.Sprintf(`{"id":%d,"name":"p%d","active":true}`, i, i))
			}
		} else {
			rows = append(rows, `{"id":9001,"name":"last","active":true}`)
		}
		fmt.Fprintf(w, "[%s]", strings.Join(rows, ","))
	})

	projects, err := (&WorkspaceClient{client: client}).ListProjects(5)
	if err != nil {
		t.Fatal(err)
	}

	if projects.Count != projectsPerPage+1 {
		t.Errorf("got %d projects, want %d", projects.Count, projectsPerPage+1)
	}
	if len(pages) != 2 || pages[0] != "1" || pages[1] != "2" {
		t.Errorf("requested pages %v, want [1 2]", pages)
	}
	if last := projects.Projects[projects.Count-1]; last.Name != "last" {
		t.Errorf("last project = %q, want the one from page 2", last.Name)
	}
}

func TestListProjectsWhereAsksForActiveOnly(t *testing.T) {
	var query url.Values

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		fmt.Fprint(w, `[{"id":91,"name":"SW","active":true}]`)
	})

	_, err := (&WorkspaceClient{client: client}).ListProjectsWhere(5,
		ProjectQuery{ActiveOnly: true})
	if err != nil {
		t.Fatal(err)
	}

	if query.Get("active") != "true" {
		t.Errorf("active = %q, want true", query.Get("active"))
	}
	if query.Get("per_page") != strconv.Itoa(projectsPerPage) {
		t.Errorf("per_page = %q, want %d", query.Get("per_page"), projectsPerPage)
	}
}

// An unfiltered listing must not quietly ask for active projects only.
func TestListProjectsDoesNotFilter(t *testing.T) {
	var query url.Values

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		fmt.Fprint(w, `[{"id":91,"name":"SW","active":false}]`)
	})

	projects, err := (&WorkspaceClient{client: client}).ListProjects(5)
	if err != nil {
		t.Fatal(err)
	}

	if query.Has("active") {
		t.Errorf("unfiltered listing sent active=%q", query.Get("active"))
	}
	if projects.Count != 1 || projects.Projects[0].Active {
		t.Errorf("archived project was dropped: %+v", projects.Projects)
	}
}
