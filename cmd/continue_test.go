package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/fefeme/workingon/toggl"
)

func recentThree() ([]toggl.TimeEntry, []string) {
	at := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)

	entries := []toggl.TimeEntry{
		{Id: 1, Description: "DBQ import", Start: &at},
		{Id: 2, Description: "Parser review", Start: &at},
		{Id: 3, Description: "Standup", Start: &at},
	}

	return entries, []string{"DBQ import", "Parser review", "Standup"}
}

// With nobody to ask, one match is an answer: there is nothing left to decide,
// so refusing to act on it would be pedantry rather than caution.
func TestTheOnlyOneContinuesASingleMatch(t *testing.T) {
	entries, labels := recentThree()

	got, err := theOnlyOne(entries, labels, []int{1}, "parser")
	if err != nil {
		t.Fatalf("theOnlyOne: %v", err)
	}

	if got.Id != 2 {
		t.Errorf("continued entry %d, want the parser review", got.Id)
	}
}

// Several matches is a question, and a run with nobody to ask cannot put it -
// so it says what it found instead of picking one.
func TestTheOnlyOneRefusesToGuessBetweenSeveral(t *testing.T) {
	entries, labels := recentThree()

	_, err := theOnlyOne(entries, labels, []int{0, 1, 2}, "e")

	if err == nil {
		t.Fatal("one of three was picked without asking")
	}
	for _, want := range []string{"3 recent entries match", `"e"`, "DBQ import"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
}

// Without a query there is nothing to narrow by, and the error should say so
// rather than complaining about a match that was never asked for.
func TestTheOnlyOneAsksForAQueryWhenThereIsNone(t *testing.T) {
	entries, labels := recentThree()

	_, err := theOnlyOne(entries, labels, []int{0, 1, 2}, "")

	if err == nil {
		t.Fatal("one of three was picked without asking")
	}
	if !strings.Contains(err.Error(), "give a query") {
		t.Errorf("error = %q, want it to ask for a query", err)
	}
}

// A long list is summarised rather than printed in full: an error message is
// not a listing.
func TestTheOnlyOneSummarisesALongList(t *testing.T) {
	at := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)

	var entries []toggl.TimeEntry
	var labels []string
	var matched []int

	for i := 0; i < 12; i++ {
		entries = append(entries, toggl.TimeEntry{Id: i + 1, Start: &at})
		labels = append(labels, "Something")
		matched = append(matched, i)
	}

	_, err := theOnlyOne(entries, labels, matched, "s")

	if err == nil {
		t.Fatal("one of twelve was picked without asking")
	}
	if !strings.Contains(err.Error(), "and 9 more") {
		t.Errorf("error = %q, want the rest summarised", err)
	}
}

// The row says when the work was last done, which is how two days of the same
// thing are told apart.
func TestRecentChoiceSaysWhenItWasLastDone(t *testing.T) {
	cfg := modifyConfig()
	at := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
	entry := toggl.TimeEntry{Description: "DBQ import", Start: &at}

	got := recentChoice(&entry, "DBQ import", cfg)

	if !strings.Contains(got, "DBQ import") || !strings.Contains(got, "18.8.2026 09:00") {
		t.Errorf("choice = %q, want the work and when it was", got)
	}
}

// An entry with no start is still worth offering - it just cannot say when.
func TestRecentChoiceWithoutAStart(t *testing.T) {
	entry := toggl.TimeEntry{Description: "DBQ import"}

	if got := recentChoice(&entry, "DBQ import", modifyConfig()); got != "DBQ import" {
		t.Errorf("choice = %q, want the label on its own", got)
	}
}

// Stopping used to print the entry itself, which is Go's idea of a time: a UTC
// timestamp with the offset on the end. What somebody wants to read is the
// afternoon they just booked, in their own zone.
func TestRenderStoppedSpeaksInTheUsersZone(t *testing.T) {
	cfg := modifyConfig()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Settings.Location = *berlin

	start := time.Date(2026, 8, 19, 13, 50, 0, 0, time.UTC)
	stop := start.Add(90 * time.Minute)
	entry := &toggl.TimeEntry{
		Description: "DBQ import", Start: &start, Stop: &stop, Duration: 5400,
	}

	out := renderStopped(entry, cfg)

	// 13:50 UTC is 15:50 in Berlin, which is where the work happened.
	if !strings.Contains(out, "19.8.2026 15:50") {
		t.Errorf("output does not show the local time:\n%s", out)
	}
	for _, technical := range []string{"UTC", "+0000", "+02:00", "T13:50"} {
		if strings.Contains(out, technical) {
			t.Errorf("output leaks the technical form %q:\n%s", technical, out)
		}
	}
}
