// Package tokens provides a cheap, dependency-free estimate of how many tokens a
// byte payload will cost a model, used only to decide whether to optimize.
package tokens

// charsPerToken is the rough divisor used for the heuristic estimate.
const charsPerToken = 4

// Estimate returns an approximate token count for b using a chars/token heuristic.
func Estimate(b []byte) int {
	return len(b) / charsPerToken
}

// Exceeds reports whether b's estimated token count is strictly greater than threshold.
func Exceeds(b []byte, threshold int) bool {
	return Estimate(b) > threshold
}
