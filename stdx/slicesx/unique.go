package slicesx

func Unique[T comparable](input []T) []T {
	inResult := make(map[T]struct{}, len(input))
	// we try to reduce allocations by using a default capacity
	result := make([]T, 0, len(input)/4)

	for _, elem := range input {
		if _, ok := inResult[elem]; !ok {
			inResult[elem] = struct{}{}
			result = append(result, elem)
		}
	}

	return result
}
