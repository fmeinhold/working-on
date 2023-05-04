package toggl_api

import "encoding/json"

type WorkspacesClient struct {
	config *RequestConfig
}

type Workspace struct {
	Id      int    `json:"id"`
	Name    string `json:"name"`
	Premium bool   `json:"premium"`
}

func (w *WorkspacesClient) List() ([]Workspace, error) {
	rawMessage, err := w.config.SendRequest("GET", "workspaces", nil)
	if err != nil {
		return nil, err
	}

	var workspaces []Workspace
	err = json.Unmarshal(*rawMessage, &workspaces)
	if err != nil {
		return nil, err
	}
	return workspaces, nil
}

func (*WorkspacesClient) get(wid int) (*Workspace, error) {
	return nil, nil
}
