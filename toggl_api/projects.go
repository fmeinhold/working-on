package toggl_api

import (
	"encoding/json"
	"fmt"
	"net/url"
)

type ProjectsClient struct {
	config *RequestConfig
}

type Project struct {
	Id          int    `json:"id"`
	Name        string `json:"name"`
	Premium     bool   `json:"premium"`
	Active      bool   `json:"active"`
	WorkspaceID int    `json:"workspace_id"`
}

func (c *ProjectsClient) List(workspaceID int) ([]Project, error) {
	queryParams := url.Values{}
	queryParams.Set("active", "true")

	rawMessage, err := c.config.SendRequest("GET", fmt.Sprintf("workspaces/%d/projects?%s", workspaceID, queryParams.Encode()), nil)
	if err != nil {
		return nil, err
	}
	var projects []Project
	json.Unmarshal(*rawMessage, &projects)

	return projects, err
}
