package slicesx_test

import (
	"slices"
	"testing"

	"markdown.ninja/stdx/slicesx"
	"markdown.ninja/stdx/uuid"
)

func TestUniqueUUIDs(t *testing.T) {
	uuid1 := uuid.NewV4()
	uuid2 := uuid.NewV4()
	uuid3 := uuid.NewV4()
	uuid4 := uuid.NewV4()
	uuid5 := uuid.NewV4()

	input := [][]uuid.UUID{
		{},
		{uuid1},
		{uuid1, uuid1},
		{uuid1, uuid1, uuid2},
		{uuid1, uuid1, uuid2, uuid2, uuid1, uuid3},
		{uuid1, uuid2, uuid3, uuid4, uuid5},
	}
	expected := [][]uuid.UUID{
		{},
		{uuid1},
		{uuid1},
		{uuid1, uuid2},
		{uuid1, uuid2, uuid3},
		{uuid1, uuid2, uuid3, uuid4, uuid5},
	}

	for i := range input {
		output := slicesx.Unique(input[i])
		if !slices.EqualFunc(expected[i], output, func(a, b uuid.UUID) bool {
			return a.String() == b.String()
		}) {
			t.Errorf("(%d) %#v (output) != %#v (expected)", i, output, expected[i])
		}
	}

}
