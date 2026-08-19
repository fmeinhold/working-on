package cmd

import (
	"sort"
	"strings"
	"unicode"
)

// fuzzyMatches ranks names against a query, best first, dropping the ones it
// does not match at all.
//
// A query matches where its letters appear in order rather than together, so
// "dbqim" finds "DBQ import" and "lp3 oauth" finds "LP3-412: DBQ OAuth app" -
// which is the point: you remember what the work was, not how you spelled it
// at four in the afternoon.
//
// Ties keep the order they came in, so an equally good match on something you
// did this morning beats one from three weeks ago.
func fuzzyMatches(names []string, query string) []int {
	query = strings.TrimSpace(query)

	found := make([]int, 0, len(names))
	if query == "" {
		for i := range names {
			found = append(found, i)
		}
		return found
	}

	type scored struct {
		index int
		score int
	}

	var ranked []scored
	for i, name := range names {
		if score, ok := fuzzyScore(name, query); ok {
			ranked = append(ranked, scored{index: i, score: score})
		}
	}

	sort.SliceStable(ranked, func(a, b int) bool {
		return ranked[a].score > ranked[b].score
	})

	for _, match := range ranked {
		found = append(found, match.index)
	}

	return found
}

// Scoring, in the order it matters: letters that ran together in the name are
// worth more than scattered ones, and a letter that began a word is worth more
// than one in the middle of it. Whitespace in the query is ignored, so "lp3
// oauth" and "lp3oauth" are the same question.
const (
	adjacentBonus = 8
	boundaryBonus = 6
	spreadPenalty = 1
)

// fuzzyScore reports how well a name answers a query, and whether it answers
// it at all. The scan is greedy - it takes the first letter it can - which can
// miss the best alignment in a name that repeats itself, but never turns a
// match into a miss.
func fuzzyScore(name, query string) (int, bool) {
	haystack := []rune(strings.ToLower(name))
	needle := []rune(strings.ToLower(strings.Join(strings.Fields(query), "")))

	score, at := 0, 0
	previous := -2

	for _, want := range needle {
		found := -1
		for i := at; i < len(haystack); i++ {
			if haystack[i] == want {
				found = i
				break
			}
		}

		if found < 0 {
			return 0, false
		}

		switch {
		case found == previous+1:
			score += adjacentBonus
		case atWordStart(haystack, found):
			score += boundaryBonus
		default:
			score -= spreadPenalty * (found - previous - 1)
		}

		previous = found
		at = found + 1
	}

	// A short name matching the same letters is the more likely answer: "call"
	// beats "calling the whole thing off" on a query of "call".
	score -= len(haystack) / 20

	return score, true
}

func atWordStart(haystack []rune, at int) bool {
	if at == 0 {
		return true
	}

	before := haystack[at-1]

	return !unicode.IsLetter(before) && !unicode.IsDigit(before)
}
