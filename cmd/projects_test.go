package cmd

import (
	"strings"
	"testing"

	"github.com/fefeme/workingon/workingon"
)

func TestCurrentProjectNoteNamesTheMappingThatChoseIt(t *testing.T) {
	mapping := &workingon.ProjectMapping{Name: "SW Biz Dev", TogglePid: 91210706}

	got := currentProjectNote(91210706, "SW Biz Dev", mapping, false)

	for _, want := range []string{"SW Biz Dev", "91210706", "for this repository"} {
		if !strings.Contains(got, want) {
			t.Errorf("note missing %q: %s", want, got)
		}
	}
}

func TestCurrentProjectNoteCreditsTheDefaultWhenNoMappingApplies(t *testing.T) {
	got := currentProjectNote(4242, "Fallback", nil, false)

	if !strings.Contains(got, "toggl_default_pid") {
		t.Errorf("note should credit the default setting: %s", got)
	}
}

func TestCurrentProjectNoteWithNothingSelected(t *testing.T) {
	got := currentProjectNote(0, "", nil, false)

	if !strings.Contains(got, "No project is currently selected") {
		t.Errorf("got %s", got)
	}
}

// A project can be selected and still be absent from the listing - archived,
// most often - and an unexplained marker-less footer would be a puzzle.
func TestCurrentProjectNoteWhenTheProjectIsNotListed(t *testing.T) {
	got := currentProjectNote(91210706, "", nil, false)
	if !strings.Contains(got, "not in the list above") || !strings.Contains(got, "--archived") {
		t.Errorf("note should explain the absence and suggest --archived: %s", got)
	}

	// Already listing archived projects, so that suggestion would be useless.
	got = currentProjectNote(91210706, "", nil, true)
	if strings.Contains(got, "--archived") {
		t.Errorf("note should not suggest --archived when it is already set: %s", got)
	}
}

func TestMarkerAndHighlightLeaveUnselectedRowsAlone(t *testing.T) {
	if got := highlight("SW Biz Dev", false); got != "SW Biz Dev" {
		t.Errorf("unselected text was altered: %q", got)
	}
	if got := marker(false); got != " " {
		t.Errorf("unselected marker = %q, want a blank", got)
	}
	if !strings.Contains(marker(true), "▸") {
		t.Errorf("selected marker = %q, want a pointer", marker(true))
	}
}
