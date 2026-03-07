package model

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// LocalTime 自定义时间类型，JSON 序列化为 "2006-01-02 15:04:05"
type LocalTime struct {
	time.Time
}

func (t LocalTime) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	formatted := fmt.Sprintf(`"%s"`, t.Format("2006-01-02 15:04:05"))
	return []byte(formatted), nil
}

func (t *LocalTime) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	str := string(data)
	str = str[1 : len(str)-1]
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", str, time.Local)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

func (t LocalTime) Value() (driver.Value, error) {
	if t.IsZero() {
		return nil, nil
	}
	return t.Time, nil
}

func (t *LocalTime) Scan(v interface{}) error {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case time.Time:
		t.Time = val
	case string:
		parsed, err := time.ParseInLocation("2006-01-02 15:04:05", val, time.Local)
		if err != nil {
			return err
		}
		t.Time = parsed
	}
	return nil
}
