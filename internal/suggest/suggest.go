// Package suggest finds the closest candidate to a misspelled word.
package suggest

// Closest returns the candidate with the smallest edit distance to word, if
// that distance is small enough to be a plausible typo. It returns "" otherwise.
// Ties are broken by candidate order.
func Closest(word string, candidates []string) string {
	best, bestD := "", -1
	for _, c := range candidates {
		d := Distance(word, c)
		if bestD < 0 || d < bestD {
			best, bestD = c, d
		}
	}
	if best == "" {
		return ""
	}
	limit := len(word) / 3
	if limit < 1 {
		limit = 1
	}
	if limit > 3 {
		limit = 3
	}
	if bestD > limit {
		return ""
	}
	return best
}

// Distance is the Damerau-Levenshtein (optimal string alignment) distance.
func Distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	n, m := len(ra), len(rb)
	d := make([][]int, n+1)
	for i := range d {
		d[i] = make([]int, m+1)
		d[i][0] = i
	}
	for j := 0; j <= m; j++ {
		d[0][j] = j
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[n][m]
}
