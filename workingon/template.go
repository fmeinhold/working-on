package workingon

import (
	"bytes"
	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/util"
	"text/template"
)

func (t *TemplateConfig) CreateTimeEntryFromTemplate(templateArgs map[string]string) (*toggl.TimeEntry, error) {

	tpl, err := template.New("t").Parse(t.Description)
	if err != nil {
		return nil, err
	}

	var description bytes.Buffer

	err = tpl.Execute(&description, templateArgs)
	if err != nil {
		return nil, err
	}

	timeEntry := toggl.TimeEntry{
		Description: description.String(),
		TaskId:      t.TogglTask,
		Billable:    false,
		CreatedWith: toggl.CreatedWith,
	}

	if t.Start != "" {
		start, err := util.ParseTimeUTCE(t.Start, Configuration.Settings.DateLayout,
			Configuration.Settings.DateTimeLayout, &Configuration.Settings.Location)
		if err != nil {
			return nil, err
		}
		timeEntry.Start = &start
	}
	if t.Stop != "" {
		stop, err := util.ParseTimeUTCE(t.Stop, Configuration.Settings.DateLayout,
			Configuration.Settings.DateTimeLayout, &Configuration.Settings.Location)
		if err != nil {
			return nil, err
		}
		timeEntry.Stop = &stop
	}

	if t.TogglTask > 0 {
		timeEntry.TaskId = t.TogglTask
	}

	if timeEntry.Stop != nil && timeEntry.Duration == 0 {
		timeEntry.Duration = timeEntry.Stop.Sub(*timeEntry.Start).Milliseconds() / 1000
	}

	return &timeEntry, nil

}

func stringArrayToInterface(list []string) []interface{} {
	values := make([]interface{}, len(list))
	for i, v := range list {
		values[i] = v
	}
	return values
}
