package guid

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"markdown.ninja/stdx/base32"
	"markdown.ninja/stdx/crypto"
	"markdown.ninja/stdx/uuid"
)

const (
	Size = 16
)

// A GUID is a 128 bit (16 byte) Globally Unique IDentifier
type GUID [Size]byte

var (
	ErrGuidIsNotValid = errors.New("GUID is not valid")
	ErrUuidIsNotValid = errors.New("Not a valid UUID")
)

var (
	Empty GUID // empty GUID, all zeros
)

func NewRandom() GUID {
	uuid := uuid.NewV4()
	return GUID(uuid)
}

func NewTimeBased() GUID {
	uuid := uuid.NewV7()
	return GUID(uuid)
}

// NewFormTime generates a new time-based guid from the given time
func NewFromTime(time time.Time) GUID {
	uuid := uuid.NewV7FromTime(time)
	return GUID(uuid)
}

// TODO: parse without allocs
func Parse(input string) (guid GUID, err error) {
	bytes, err := base32.DecodeString(input)
	if err != nil {
		err = ErrGuidIsNotValid
		return
	}

	if len(bytes) != Size {
		err = ErrGuidIsNotValid
		return
	}

	return GUID(bytes), nil
}

// FromBytes creates a new GUID from a byte slice. Returns an error if the slice
// does not have a length of 16. The bytes are copied from the slice.
func FromBytes(b []byte) (guid GUID, err error) {
	err = guid.UnmarshalBinary(b)
	return guid, err
}

// String returns the string form of guid
// TODO: encode without alloc
func (guid GUID) String() string {
	return base32.EncodeToString(guid[:])
}

func (guid GUID) Equal(other GUID) bool {
	return crypto.ConstantTimeCompare(guid[:], other[:])
}

func (guid GUID) Bytes() []byte {
	return guid[:]
}

// MarshalText implements encoding.TextMarshaler.
func (guid GUID) MarshalText() ([]byte, error) {
	ret := guid.String()
	return []byte(ret), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (guid *GUID) UnmarshalText(data []byte) error {
	id, err := Parse(string(data))
	if err != nil {
		return err
	}
	*guid = id
	return nil
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (guid GUID) MarshalBinary() ([]byte, error) {
	return guid[:], nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (guid *GUID) UnmarshalBinary(data []byte) error {
	if len(data) != 16 {
		return fmt.Errorf("invalid GUID (got %d bytes)", len(data))
	}
	copy(guid[:], data)
	return nil
}

// Scan implements sql.Scanner so GUIDs can be read from databases transparently.
// Currently, database types that map to string and []byte are supported. Please
// consult database-specific driver documentation for matching types.
func (guid *GUID) Scan(src interface{}) error {
	switch src := src.(type) {
	case nil:
		return nil

	case string:
		// if an empty GUID comes from a table, we return a null GUID
		if src == "" {
			return nil
		}

		// see Parse for required string format
		u, err := ParseUuidString(src)
		if err != nil {
			return fmt.Errorf("Scan: %v", err)
		}

		*guid = u

	case []byte:
		// if an empty GUID comes from a table, we return a null GUID
		if len(src) == 0 {
			return nil
		}

		// assumes a simple slice of bytes if 16 bytes
		// otherwise attempts to parse
		if len(src) != 16 {
			return guid.Scan(string(src))
		}
		copy((*guid)[:], src)

	default:
		return fmt.Errorf("Scan: unable to scan type %T into GUID", src)
	}

	return nil
}

// Value implements sql.Valuer so that GUIDs can be written to databases
// transparently. Currently, GUIDs map to []byte. Please consult
// database-specific driver documentation for matching types.
func (guid GUID) Value() (driver.Value, error) {
	return guid[:], nil
}

func (guid GUID) IsNil() bool {
	return guid.Equal(Empty)
}
