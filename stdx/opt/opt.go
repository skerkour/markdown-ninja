// package opt provides optional types. It can be useful for optional configuration paramteters
package opt

type Option[Value any] struct {
	value  Value
	exists bool
}

func None[V any]() Option[V] {
	return Option[V]{
		exists: false,
	}
}

func Some[V any](value V) Option[V] {
	return Option[V]{
		value:  value,
		exists: true,
	}
}

func (o Option[Value]) Get() (Value, bool) {
	return o.value, o.exists
}
