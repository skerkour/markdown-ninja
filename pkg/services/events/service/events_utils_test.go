package service

import (
	"encoding/binary"
	"net/netip"
	"testing"
	"time"

	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/xxh3"
)

func BenchmarkGetAnonymousID(b *testing.B) {
	ipAddress, _ := netip.ParseAddr("0.0.0.0")
	input := getAnonymousIdInput{
		time:      time.Now().UTC(),
		websiteID: guid.NewTimeBased(),
		IpAddress: ipAddress,
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 Edg/126.0.0.0",
	}

	b.Run("BLAKE3", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(input.IpAddress.AsSlice()) + len(input.UserAgent)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			getAnonymousID("not random salt", input)
		}
	})

	b.Run("XXH3", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(input.IpAddress.AsSlice()) + len(input.UserAgent)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			getAnonymousIDXxh3(input)
		}
	})
}

func getAnonymousIDXxh3(input getAnonymousIdInput) (anonymousID guid.GUID) {
	year, month, day := input.time.UTC().Date()
	var dateBuffer [4]byte
	binary.BigEndian.PutUint16(dateBuffer[:], uint16(year))
	dateBuffer[2] = uint8(month)
	dateBuffer[3] = uint8(day)

	// dateSeed := uint64(uint32(year)<<16 | uint32(month)<<8 | uint32(day))

	hasher := xxh3.New()
	hasher.Write(dateBuffer[:])
	hasher.Write(input.websiteID.Bytes())
	hasher.Write([]byte(input.IpAddress.AsSlice()))
	hasher.Write([]byte(input.UserAgent))

	anonymousID = guid.GUID(hasher.Sum128().Bytes())
	return
}
