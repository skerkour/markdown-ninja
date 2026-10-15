package slicesx

func Map[I, O any](input []I, fn func(elem I, index int) O) []O {
	ret := make([]O, len(input))

	for i, elem := range input {
		ret[i] = fn(elem, i)
	}

	return ret
}
