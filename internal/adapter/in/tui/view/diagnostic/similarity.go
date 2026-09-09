package diagnostic

import "math"

// FindModelSuggestions finds nearest matching model names from known catalogs using Levenshtein distance.
func levenshteinDistance(s1, s2 string) int {
	r1, r2 := []rune(s1), []rune(s2)
	l1, l2 := len(r1), len(r2)
	matrix := make([][]int, l1+1)
	for i := range matrix {
		matrix[i] = make([]int, l2+1)
		matrix[i][0] = i
	}
	for j := 0; j <= l2; j++ {
		matrix[0][j] = j
	}

	for i := 1; i <= l1; i++ {
		for j := 1; j <= l2; j++ {
			cost := 1
			if r1[i-1] == r2[j-1] {
				cost = 0
			}
			matrix[i][j] = int(math.Min(
				float64(matrix[i-1][j]+1),
				math.Min(
					float64(matrix[i][j-1]+1),
					float64(matrix[i-1][j-1]+cost),
				),
			))
		}
	}
	return matrix[l1][l2]
}
