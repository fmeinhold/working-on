package cmd

import (
	"strings"
	"testing"

	"github.com/fefeme/workingon/workingon"
)

func noLabels() templateLabels {
	return templateLabels{projects: map[int]string{}, tasks: map[int]string{}}
}

// The alias is what you type, so it leads the row, and the description says
// what typing it books.
func TestRenderTemplatesListsAliasAndDescription(t *testing.T) {
	templates := []workingon.TemplateConfig{
		{Alias: "ds", Description: "Daily Standup"},
		{Alias: "call", Description: "Call with {{.caller}}"},
	}

	got := renderTemplates(templates, noLabels())

	for _, want := range []string{"ds", "Daily Standup", "call", "Call with {{.caller}}", "2 templates"} {
		if !strings.Contains(got, want) {
			t.Errorf("listing missing %q:\n%s", want, got)
		}
	}
}

// Config order is what the user wrote, so it is the order they are shown in.
func TestRenderTemplatesKeepsConfigOrder(t *testing.T) {
	templates := []workingon.TemplateConfig{
		{Alias: "zulu", Description: "Last"},
		{Alias: "alpha", Description: "First"},
	}

	got := renderTemplates(templates, noLabels())

	if strings.Index(got, "zulu") > strings.Index(got, "alpha") {
		t.Errorf("listing was re-ordered:\n%s", got)
	}
}

// A pinned project and task are named where the lookup had an answer.
func TestRenderTemplatesNamesThePinnedProjectAndTask(t *testing.T) {
	templates := []workingon.TemplateConfig{
		{Alias: "ds", Description: "Daily Standup", TogglPid: 91210706, TogglTask: 241929955},
	}
	labels := templateLabels{
		projects: map[int]string{91210706: "SW Biz Dev"},
		tasks:    map[int]string{241929955: "ATD Conference"},
	}

	got := renderTemplates(templates, labels)

	for _, want := range []string{"SW Biz Dev", "ATD Conference"} {
		if !strings.Contains(got, want) {
			t.Errorf("listing missing %q:\n%s", want, got)
		}
	}
}

// Offline, or against a project that has since gone, the id stands for itself
// rather than the listing failing or showing a blank.
func TestRenderTemplatesFallsBackToIdsWithoutNames(t *testing.T) {
	templates := []workingon.TemplateConfig{
		{Alias: "ds", Description: "Daily Standup", TogglPid: 91210706, TogglTask: 241929955},
	}

	got := renderTemplates(templates, noLabels())

	for _, want := range []string{"#91210706", "#241929955"} {
		if !strings.Contains(got, want) {
			t.Errorf("listing missing %q:\n%s", want, got)
		}
	}
}

// A template that pins nothing carries no trailing detail at all.
func TestRenderTemplatesLeavesAnUnpinnedTemplateBare(t *testing.T) {
	templates := []workingon.TemplateConfig{{Alias: "call", Description: "A call"}}

	got := renderTemplates(templates, noLabels())

	if strings.Contains(got, "·") {
		t.Errorf("listing added detail to a template that pins nothing:\n%s", got)
	}
}

// The details line up in a column of their own, so a screenful of templates
// reads down as well as across.
func TestRenderTemplatesAlignsTheDetailColumn(t *testing.T) {
	templates := []workingon.TemplateConfig{
		{Alias: "ds", Description: "Daily Standup", Start: "17:30", Stop: "17:45"},
		{Alias: "invoicing", Description: "Invoicing", TogglPid: 77918943},
	}

	var columns []int
	for _, line := range strings.Split(renderTemplates(templates, noLabels()), "\n") {
		if at := strings.Index(line, "·"); at >= 0 {
			columns = append(columns, at)
		}
	}

	if len(columns) != 2 {
		t.Fatalf("found %d detail rows, want 2", len(columns))
	}
	if columns[0] != columns[1] {
		t.Errorf("details start at columns %d and %d, want them aligned", columns[0], columns[1])
	}
}

func TestTemplateWhen(t *testing.T) {
	cases := map[string]struct {
		template workingon.TemplateConfig
		want     string
	}{
		"both ends":  {workingon.TemplateConfig{Start: "17:30", Stop: "17:45"}, "17:30-17:45"},
		"start only": {workingon.TemplateConfig{Start: "17:30"}, "from 17:30"},
		"stop only":  {workingon.TemplateConfig{Stop: "17:45"}, "until 17:45"},
		"neither":    {workingon.TemplateConfig{}, ""},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := templateWhen(tc.template); got != tc.want {
				t.Errorf("templateWhen = %q, want %q", got, tc.want)
			}
		})
	}
}

// An empty list is not an error - it is a config that has none yet, and saying
// where they go beats an empty screen.
func TestRenderTemplatesWithNoneConfigured(t *testing.T) {
	got := renderTemplates(nil, noLabels())

	if !strings.Contains(got, "No templates configured") {
		t.Errorf("got %s", got)
	}
	if !strings.Contains(got, "templates:") {
		t.Errorf("message should say where they go:\n%s", got)
	}
}

// Nothing to name means nothing to look up, so the listing never calls out for
// a set of templates that pins neither a project nor a task.
func TestLookupTemplateLabelsSkipsTheRoundTripWhenNothingIsPinned(t *testing.T) {
	labels := lookupTemplateLabels([]workingon.TemplateConfig{
		{Alias: "call", Description: "A call"},
	})

	if len(labels.projects) != 0 || len(labels.tasks) != 0 {
		t.Errorf("labels = %+v, want nothing looked up", labels)
	}
}

func TestCountOfTemplates(t *testing.T) {
	if got := countOfTemplates(1); got != "1 template" {
		t.Errorf("countOfTemplates(1) = %q", got)
	}
	if got := countOfTemplates(3); got != "3 templates" {
		t.Errorf("countOfTemplates(3) = %q", got)
	}
}
