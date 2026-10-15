package guid_test

import (
	"testing"

	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/uuid"
)

func TestParseUuidString(t *testing.T) {
	for i := 0; i < 10000; i += 1 {
		id := guid.NewRandom()
		if id.Equal(guid.Empty) {
			t.Error("GUID is empty")
		}

		parsed, err := guid.ParseUuidString(id.ToUuidString())
		if err != nil {
			t.Errorf("parsing GUID: %s", err)
		}
		if !id.Equal(parsed) {
			t.Errorf("parsed (%s) != original GUID (%s)", parsed.String(), id.String())
		}
	}
}

func TestNewRandomIsValidUuidV4(t *testing.T) {
	for i := 0; i < 10000; i += 1 {
		id := guid.NewRandom()
		if id.Equal(guid.Empty) {
			t.Error("GUID is empty")
		}

		parsedUuid, err := uuid.FromBytes(id.Bytes())
		if err != nil {
			t.Errorf("parsing UUID: %s", err)
		}

		if parsedUuid.Version() != 4 {
			t.Error("UUID is not v4")
		}

		if parsedUuid.Variant() != uuid.RFC4122 {
			t.Error("UUID is not RFC4122")
		}
	}
}

func TestNewTimebasedIsValidUuidV7(t *testing.T) {
	for i := 0; i < 10000; i += 1 {
		id := guid.NewTimeBased()
		if id.Equal(guid.Empty) {
			t.Error("GUID is empty")
		}

		parsedUuid, err := uuid.FromBytes(id.Bytes())
		if err != nil {
			t.Errorf("parsing UUID: %s", err)
		}

		if parsedUuid.Version() != 7 {
			t.Error("UUID is not v7")
		}

		if parsedUuid.Variant() != uuid.RFC4122 {
			t.Error("UUID is not RFC4122")
		}
	}
}
