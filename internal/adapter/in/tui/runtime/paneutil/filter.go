package paneutil

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/list"
	"github.com/sahilm/fuzzy"
)

type candidate struct {
	index          int
	score          int
	matchedIndexes []int
}

// SmartFilter provides multi-word, prefix-prioritized, typo-tolerant search filtering.
func SmartFilter(term string, targets []string) []list.Rank {
	cleanTerm := strings.TrimSpace(strings.ToLower(term))
	if cleanTerm == "" {
		res := make([]list.Rank, len(targets))
		for i := range targets {
			res[i] = list.Rank{Index: i}
		}
		return res
	}

	tokens := strings.Fields(cleanTerm)
	candidates := make([]candidate, 0, len(targets))

	for i, target := range targets {
		lowerTarget := strings.ToLower(target)
		allMatched := true
		itemScore := 0
		var matchedIndices []int

		if lowerTarget == cleanTerm {
			itemScore += 100000
		} else if strings.HasPrefix(lowerTarget, cleanTerm) {
			itemScore += 50000
		}

		targetWords := strings.FieldsFunc(lowerTarget, func(r rune) bool {
			return r == ' ' || r == '-' || r == '_' || r == '/' || r == '.' || r == ':'
		})

		for _, token := range tokens {
			tokenMatched := false
			bestTokenScore := 0
			var tokenIndices []int

			for wIdx, word := range targetWords {
				if word == token {
					score := 5000 - min(wIdx*10, 500)
					if score > bestTokenScore {
						bestTokenScore = score
					}
					tokenMatched = true
				} else if strings.HasPrefix(word, token) {
					score := 3000 - min(wIdx*10, 500)
					if score > bestTokenScore {
						bestTokenScore = score
					}
					tokenMatched = true
				}
			}

			if idx := strings.Index(lowerTarget, token); idx >= 0 {
				score := 2000 - min(idx*5, 500)
				if score > bestTokenScore {
					bestTokenScore = score
				}
				tokenMatched = true
				for k := 0; k < len(token); k++ {
					tokenIndices = append(tokenIndices, idx+k)
				}
			}

			if !tokenMatched {
				fuzzyMatches := fuzzy.Find(token, []string{lowerTarget})
				if len(fuzzyMatches) > 0 && len(fuzzyMatches[0].MatchedIndexes) > 0 {
					bestTokenScore = 500
					tokenMatched = true
					tokenIndices = append(tokenIndices, fuzzyMatches[0].MatchedIndexes...)
				}
			}

			if !tokenMatched {
				allMatched = false
				break
			}

			itemScore += bestTokenScore
			matchedIndices = append(matchedIndices, tokenIndices...)
		}

		if allMatched {
			itemScore += max(0, 500-len(lowerTarget))
			candidates = append(candidates, candidate{
				index:          i,
				score:          itemScore,
				matchedIndexes: deduplicateIndices(matchedIndices),
			})
			continue
		}

		fuzzyResults := fuzzy.Find(cleanTerm, []string{lowerTarget})
		if len(fuzzyResults) > 0 {
			candidates = append(candidates, candidate{
				index:          i,
				score:          fuzzyResults[0].Score,
				matchedIndexes: fuzzyResults[0].MatchedIndexes,
			})
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	ranks := make([]list.Rank, len(candidates))
	for i, c := range candidates {
		ranks[i] = list.Rank{
			Index:          c.index,
			MatchedIndexes: c.matchedIndexes,
		}
	}
	return ranks
}

func deduplicateIndices(indices []int) []int {
	if len(indices) <= 1 {
		return indices
	}
	sort.Ints(indices)
	out := make([]int, 0, len(indices))
	prev := -1
	for _, idx := range indices {
		if idx != prev {
			out = append(out, idx)
			prev = idx
		}
	}
	return out
}
