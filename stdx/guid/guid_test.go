package guid

import (
	"testing"
)

func TestNewTimeBased(t *testing.T) {
	for i := 0; i < 10000; i += 1 {
		id := NewTimeBased()
		if id.Equal(Empty) {
			t.Error("GUID is empty")
		}
	}
}

func TestNewRandom(t *testing.T) {
	for i := 0; i < 10000; i += 1 {
		id := NewRandom()
		if id.Equal(Empty) {
			t.Error("GUID is empty")
		}
	}
}

func TestEqual(t *testing.T) {
	for i := 0; i < 10000; i += 1 {
		id := NewRandom()
		id2 := NewRandom()

		if id.Equal(Empty) {
			t.Error("GUID is empty")
		}

		if !id.Equal(id) {
			t.Error("Equal(GUID) == false")
		}

		if id.Equal(id2) {
			t.Error("Equal(GUID_2) == true")
		}
	}
}

func TestParse(t *testing.T) {
	for i := 0; i < 10000; i += 1 {
		id := NewRandom()
		parsed, err := Parse(id.String())
		if err != nil {
			t.Errorf("parsing GUID: %s", err)
		}
		if !id.Equal(parsed) {
			t.Errorf("parsed (%s) != original GUID (%s)", parsed.String(), id.String())
		}
	}
}

// func BenchmarkNewRandomReader(b *testing.B) {
// 	numberOfGoroutines := []int{
// 		10,
// 		500,
// 		1000,
// 		2000,
// 		10_000,
// 	}
// 	goMaxProc := runtime.GOMAXPROCS(0)
// 	for _, n := range numberOfGoroutines {
// 		b.Run(fmt.Sprintf("goroutines-%d", n*goMaxProc), func(b *testing.B) {
// 			b.SetParallelism(n)
// 			b.RunParallel(func(pb *testing.PB) {
// 				for pb.Next() {
// 					id, err := newRandomFromReader(randomSource)
// 					if err != nil {
// 						b.Error(err)
// 					}
// 					_ = id
// 				}
// 			})
// 		})
// 	}
// }
