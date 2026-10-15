package timex

import "time"

func Max(x, y time.Time, times ...time.Time) time.Time {
	max := x
	if y.After(max) {
		max = y
	}

	for _, t := range times {
		if t.After(max) {
			max = t
		}
	}

	return max
}
