package iterx

import (
	"maps"
	"slices"
	"testing"
)

func TestMap(t *testing.T) {
	input := []int64{1, 2, 3, 4}
	expected := []int64{2, 4, 6, 8}

	resIterator := Map(slices.Values(input), func(x int64) int64 {
		return x * 2
	})
	res := slices.Collect(resIterator)

	if !slices.Equal(expected, res) {
		t.Errorf("Invalid result. Expected: %v | Got: %v", expected, res)
	}
}

func TestMap2(t *testing.T) {
	input := map[int64]int64{
		1: 1,
		2: 2,
		3: 3,
		4: 4,
	}
	expected := map[int64]int64{
		1: 2,
		2: 4,
		3: 6,
		4: 8,
	}

	resIterator := Map2(maps.All(input), func(key, value int64) (int64, int64) {
		return key, value * 2
	})
	res := maps.Collect(resIterator)

	if !maps.Equal(expected, res) {
		t.Errorf("Invalid result. Expected: %v | Got: %v", expected, res)
	}
}
