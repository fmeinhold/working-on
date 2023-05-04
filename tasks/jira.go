package tasks

import (
	"fmt"
	jira "github.com/andygrunwald/go-jira"
	"github.com/fmeinhold/workingon/cfg"
)

type JiraSource struct {
	client *jira.Client
}

func NewJiraSource() (*JiraSource, error) {

	tp := jira.BasicAuthTransport{
		Username: cfg.GlobalConfig.GetString("jira.username"),

		Password: cfg.GlobalConfig.GetString("jira.api_token"),
	}

	client, err := jira.NewClient(tp.Client(), "https://sealworks.atlassian.net")
	if err != nil {
		return nil, err
	}

	return &JiraSource{client: client}, nil
}

func (js *JiraSource) FetchTasks(prefix string) ([]Task, error) {
	last := 0
	var tasks []Task
	opt := &jira.SearchOptions{
		MaxResults: 1000, // Max results can go up to 1000
		StartAt:    last,
	}

	for {
		issues, resp, err := js.client.Issue.Search(fmt.Sprintf("project='%s' and assignee=currentUser() and statusCategory != Done order by key", prefix), opt)

		if err != nil {
			return nil, err
		}

		total := resp.Total
		if tasks == nil {
			tasks = make([]Task, 0, total)
		}

		for _, issue := range issues {
			tasks = append(tasks, Task{Key: issue.Key, Summary: issue.Fields.Summary})
		}

		last = resp.StartAt + len(issues)

		if last >= total {
			return tasks, nil
		}
	}
}
