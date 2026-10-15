package gostub

// TestingT is a subset of the testing.TB interface used by gostub.
type TestingT interface {
	Cleanup(func())
}

// Value replaces the value at varPtr with stubVal.
// The original value is reset at the end of the test via t.Cleanup
// or can be reset using the returned function.
func Value[T any](t TestingT, varPtr *T, stubVal T) (reset func()) {
	orig := *varPtr
	*varPtr = stubVal

	reset = func() {
		*varPtr = orig
	}
	t.Cleanup(reset)
	return reset
}
