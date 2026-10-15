package timex

import (
	"database/sql/driver"
	"fmt"
	"time"
)

type Time int64

func Now() Time {
	return Time(time.Now().UTC().UnixMilli())
}

func (t Time) Unix() int64 {
	return int64(t) / 1000
}

func (t Time) StdTime() time.Time {
	return time.Unix(0, int64(t)*1_000_000).UTC()
}

func (t *Time) Scan(val any) (err error) {
	switch v := val.(type) {
	case int64:
		*t = Time(v)
		return nil
	case string:
		tt, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return err
		}
		*t = Time(tt.UTC().UnixMilli())
		return nil
	default:
		return fmt.Errorf("Time.Scan: Unsupported type: %T", v)
	}
}

func (t Time) Value() (driver.Value, error) {
	return t, nil
}

func (t Time) MarshalText() (ret []byte, err error) {
	return t.StdTime().MarshalText()
}

func (t Time) MarshalJSON() ([]byte, error) {
	return t.StdTime().MarshalJSON()
}

func (t *Time) UnmarshalText(data []byte) (err error) {
	var tt time.Time

	err = tt.UnmarshalText(data)
	if err != nil {
		return err
	}

	*t = Time(tt.UnixMilli())
	return nil
}

func (t *Time) UnmarshalJSON(data []byte) (err error) {
	var tt time.Time

	err = tt.UnmarshalJSON(data)
	if err != nil {
		return err
	}

	*t = Time(tt.UnixMilli())
	return nil
}

func (t Time) String() string {
	ret, _ := t.MarshalText()
	return string(ret)
}
