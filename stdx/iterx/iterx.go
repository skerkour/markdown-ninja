package iterx

import "iter"

func Map[I, O any](input iter.Seq[I], fn func(elem I) O) iter.Seq[O] {
	return func(yield func(modifiedElem O) bool) {
		for elem := range input {
			if !yield(fn(elem)) {
				return
			}
		}
	}
}

func Map2[I, I2, O, O2 any](input iter.Seq2[I, I2], fn func(x I, y I2) (O, O2)) iter.Seq2[O, O2] {
	return func(yield func(modifiedX O, modifiedY O2) bool) {
		for x, y := range input {
			if !yield(fn(x, y)) {
				return
			}
		}
	}
}
