package cmd

import (
	"strings"
	"testing"
)

func TestCurrentProjectNoteNamesTheSelectedProject(t *testing.T) {
	got := currentProjectNote(91210706, "SW Biz Dev", false)

	for _, want := range []string{"SW Biz Dev", "91210706", "toggl_default_pid"} {
		if !strings.Contains(got, want) {
			t.Errorf("note missing %q: %s", want, got)
		}
	}
}

// Nothing selected means no default was set anywhere, and `wo init --local` is
// what sets one for the checkout you are standing in.
func TestCurrentProjectNoteWithNothingSelected(t *testing.T) {
	got := currentProjectNote(0, "", false)

	if !strings.Contains(got, "No project is currently selected") {
		t.Errorf("got %s", got)
	}
	if !strings.Contains(got, "wo init --local") {
		t.Errorf("note should say how to set one: %s", got)
	}
}

// A project can be selected and still be absent from the listing - archived,
// most often - and an unexplained marker-less footer would be a puzzle.
func TestCurrentProjectNoteWhenTheProjectIsNotListed(t *testing.T) {
	got := currentProjectNote(91210706, "", false)
	if !strings.Contains(got, "not in the list above") || !strings.Contains(got, "--archived") {
		t.Errorf("note should explain the absence and suggest --archived: %s", got)
	}

	// Already listing archived projects, so that suggestion would be useless.
	got = currentProjectNote(91210706, "", true)
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
