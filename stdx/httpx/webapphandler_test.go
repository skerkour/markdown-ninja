package httpx

// func TestWebapphandlerTypes(t *testing.T) {
// 	var fileInfo webappFileInfo
// 	fmt.Printf("size(webappFileInfo): %d\n", unsafe.Sizeof(fileInfo))
// }

// func TestEncodeEtagOptimized(t *testing.T) {
// 	elems := []struct {
// 		Input    string
// 		Expected string
// 		Hash     [32]byte
// 	}{
// 		{
// 			Input:    "1",
// 			Expected: "",
// 		},
// 		{
// 			Input:    "2",
// 			Expected: "",
// 		},
// 		{
// 			Input:    "3",
// 			Expected: "",
// 		},
// 		{
// 			Input:    "Hello World!",
// 			Expected: "",
// 		},
// 	}
// 	for i := range elems {
// 		elems[i].Hash = blake3.Sum256([]byte(elems[i].Input))
// 		elems[i].Expected = base64.RawURLEncoding.EncodeToString(elems[i].Hash[:])
// 	}

// 	for _, elem := range elems {
// 		value := encodeEtagOptimized(elem.Hash)
// 		if value != elem.Expected {
// 			t.Errorf("expected: %s | got: %s | input: %s\n", elem.Expected, value, elem.Input)
// 		}
// 	}
// }

// func BenchmarkEncodeEtagOptimized(b *testing.B) {
// 	hash := blake3.Sum256([]byte("Hello World!"))

// 	b.Run("strconv.Quote(base64.RawURLEncoding.EncodeToString(hash))", func(b *testing.B) {
// 		b.ReportAllocs()
// 		b.SetBytes(int64(len(hash)))
// 		b.ResetTimer()
// 		for b.Loop() {
// 			strconv.Quote(base64.RawURLEncoding.EncodeToString(hash[:]))
// 		}
// 	})

// 	b.Run("encodeEtag(hash)", func(b *testing.B) {
// 		b.ReportAllocs()
// 		b.SetBytes(int64(len(hash)))
// 		b.ResetTimer()
// 		for b.Loop() {
// 			encodeEtagOptimized(hash)
// 		}
// 	})

// 	// b.Run("toEtagPlus(hash)", func(b *testing.B) {
// 	// 	b.ReportAllocs()
// 	// 	b.SetBytes(int64(len(hash)))
// 	// 	b.ResetTimer()
// 	// 	for b.Loop() {
// 	// 		toEtagPlus(hash)
// 	// 	}
// 	// })
// }
