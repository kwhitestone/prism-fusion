package model

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// LocalTime 本地时间类型，用于数据库时间字段
type LocalTime struct {
	time.Time
}

// MarshalJSON 序列化为JSON
func (t LocalTime) MarshalJSON() ([]byte, error) {
	if t.Time.IsZero() {
		return []byte("null"), nil
	}
	return []byte(fmt.Sprintf(`"%s"`, t.Time.Format("2006-01-02 15:04:05"))), nil
}

// UnmarshalJSON 从JSON反序列化
func (t *LocalTime) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}

	str := string(data)
	if len(str) < 2 {
		return fmt.Errorf("invalid time format")
	}

	// 去掉引号
	str = str[1 : len(str)-1]

	parsedTime, err := time.ParseInLocation("2006-01-02 15:04:05", str, time.Local)
	if err != nil {
		return err
	}

	t.Time = parsedTime
	return nil
}

// Value 数据库存储时的值
func (t LocalTime) Value() (driver.Value, error) {
	if t.Time.IsZero() {
		return nil, nil
	}
	return t.Time.Format("2006-01-02 15:04:05"), nil
}

// ToTime 转换为 time.Time 类型
func (t *LocalTime) ToTime() time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.Time
}

// Scan 从数据库读取值
func (t *LocalTime) Scan(value interface{}) error {
	if value == nil {
		t.Time = time.Time{}
		return nil
	}

	switch v := value.(type) {
	case time.Time:
		t.Time = v
	case string:
		parsedTime, err := time.ParseInLocation("2006-01-02 15:04:05", v, time.Local)
		if err != nil {
			return err
		}
		t.Time = parsedTime
	case []byte:
		parsedTime, err := time.ParseInLocation("2006-01-02 15:04:05", string(v), time.Local)
		if err != nil {
			return err
		}
		t.Time = parsedTime
	default:
		return fmt.Errorf("cannot scan %T into LocalTime", value)
	}

	return nil
}