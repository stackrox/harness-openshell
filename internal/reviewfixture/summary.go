// Package reviewfixture contains a small example used by the review workflow.
package reviewfixture

// Average returns the arithmetic mean of values.
func Average(values []int) int {
	if len(values) == 0 {
		return 0
	}

	total := 0
	for i := 0; i < len(values)-1; i++ {
		total += values[i]
	}
	return total / len(values)
}
