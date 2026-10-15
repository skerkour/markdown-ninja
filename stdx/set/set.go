package set

import (
	"iter"
	"maps"
)

type Set[T comparable] map[T]struct{}

func New[T comparable]() Set[T] {
	return Set[T](make(map[T]struct{}))
}

func NewWithCapacity[T comparable](capacity uint64) Set[T] {
	return Set[T](make(map[T]struct{}, capacity))
}

func NewFromSlice[T comparable](fromList []T) Set[T] {
	ret := Set[T](make(map[T]struct{}))

	for _, item := range fromList {
		ret[item] = struct{}{}
	}

	return ret
}

func NewFromIter[T comparable](fromiter iter.Seq[T]) Set[T] {
	ret := Set[T](make(map[T]struct{}))

	for item := range fromiter {
		ret[item] = struct{}{}
	}

	return ret
}

func (set Set[T]) Contains(item T) bool {
	_, contains := set[item]
	return contains
}

func (set Set[T]) ToSlice() []T {
	ret := make([]T, len(set))

	i := 0
	for elem := range set {
		ret[i] = elem
		i += 1
	}

	return ret
}

// Iter returns an iterator yelding the elements of the set
func (set Set[T]) Iter() iter.Seq[T] {
	return func(yield func(T) bool) {
		for element := range set {
			if !yield(element) {
				return
			}
		}
	}
}

func (set Set[T]) Insert(element T) {
	set[element] = struct{}{}
}

func (set Set[T]) InsertIter(iterator iter.Seq[T]) {
	for elem := range iterator {
		set[elem] = struct{}{}
	}
}

func (set Set[T]) Delete(element T) {
	delete(set, element)
}

func (set Set[T]) Equal(target Set[T]) bool {
	return maps.Equal(set, target)
}
