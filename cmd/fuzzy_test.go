package cmd

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

// The point of matching letters in order rather than as a run: you remember
// the shape of what you were doing, not how you spelled it.
func TestFuzzyMatchesLettersInOrder(t *testing.T) {
	names := []string{
		"LP3-412: DBQ OAuth app + Basil role groups",
		"Mailpit for local email dev",
		"Learners report: district filter",
	}

	for query, want := range map[string]string{
		"oauth":    names[0],
		"dbqoauth": names[0],
		"lp3 dbq":  names[0],
		"mailpit":  names[1],
		"mlpt":     names[1],
		"district": names[2],
	} {
		matched := fuzzyMatches(names, query)
		if len(matched) == 0 {
			t.Errorf("%q matched nothing, want %q", query, want)
			continue
		}
		if got := names[matched[0]]; got != want {
			t.Errorf("%q ranked %q first, want %q", query, got, want)
		}
	}
}

// Letters that are not there in that order are not a match, however many of
// them the name happens to contain.
func TestFuzzyRefusesWhatIsNotThere(t *testing.T) {
	names := []string{"Mailpit for local email dev"}

	for _, query := range []string{"zzz", "postfix", "tipliam"} {
		if matched := fuzzyMatches(names, query); len(matched) != 0 {
			t.Errorf("%q matched %q, want no match", query, names[matched[0]])
		}
	}
}

// A run of letters is a better answer than the same letters scattered across
// the name, and a word start is better than the middle of one.
func TestFuzzyRanksTheObviousAnswerFirst(t *testing.T) {
	names := []string{
		"Deploying a big quiet update",
		"DBQ import",
	}

	matched := fuzzyMatches(names, "dbq")
	if len(matched) < 2 {
		t.Fatalf("want both names matched, got %d", len(matched))
	}
	if got := names[matched[0]]; got != "DBQ import" {
		t.Errorf("ranked %q first, want the run of letters to win", got)
	}
}

// An empty query is not a filter, and everything keeps the order it came in -
// which for the recent listing is newest first.
func TestFuzzyWithoutAQueryKeepsEveryoneInOrder(t *testing.T) {
	names := []string{"first", "second", "third"}

	matched := fuzzyMatches(names, "")

	if len(matched) != 3 {
		t.Fatalf("matched %d, want all 3", len(matched))
	}
	for i, index := range matched {
		if index != i {
			t.Errorf("matched[%d] = %d, want the order untouched", i, index)
		}
	}
}

// Two names the query answers equally well keep the order they arrived in, so
// what you did this morning beats what you did three weeks ago.
func TestFuzzyKeepsTiesInTheOrderGiven(t *testing.T) {
	names := []string{"review the parser", "review the parser"}

	matched := fuzzyMatches(names, "parser")

	if len(matched) != 2 || matched[0] != 0 {
		t.Errorf("matched = %v, want the earlier one first", matched)
	}
}

// The picker narrows with whatever the caller matches by, so the listing
// answers the same way the command line does.
func TestPickOneMatchingNarrowsFuzzily(t *testing.T) {
	names := []string{"Mailpit for local email dev", "DBQ import", "Learners report"}

	out := &bytes.Buffer{}
	prompt := &prompter{reader: bufio.NewReader(strings.NewReader("dbqim\n1\n")), out: out}

	index := pickOneMatching(prompt, "Which one", names, fuzzyMatches)

	if index != 1 {
		t.Errorf("picked %d (%q), want the DBQ import", index, names[index])
	}
	if !strings.Contains(out.String(), "matching \"dbqim\"") {
		t.Errorf("the listing was not narrowed:\n%s", out)
	}
}

// pickOne itself is unchanged: the project listing still narrows by substring,
// where "front" is meant to find the frontend and not every f, r, o, n, t.
func TestPickOneStillNarrowsBySubstring(t *testing.T) {
	names := []string{"Invoicing Frontend", "Legacy Invoices"}

	out := &bytes.Buffer{}
	prompt := &prompter{reader: bufio.NewReader(strings.NewReader("front\n1\n")), out: out}

	if index := pickOne(prompt, "Which one", names); index != 0 {
		t.Errorf("picked %q, want the frontend", names[index])
	}
	if !strings.Contains(out.String(), "1 matching \"front\"") {
		t.Errorf("substring narrowing did not happen:\n%s", out)
	}
}
