package model

import "time"

// AuthRateWindow stores a privacy-preserving fixed-window counter. KeyHash is
// derived from the scope, normalized subject, and minute bucket, so raw IPs and
// usernames are never persisted.
type AuthRateWindow struct {
	KeyHash   string    `gorm:"primaryKey;size:64" json:"-"`
	Scope     string    `gorm:"size:32;not null;index" json:"-"`
	Count     uint      `gorm:"not null" json:"-"`
	ExpiresAt time.Time `gorm:"not null;index" json:"-"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
}

func (AuthRateWindow) TableName() string { return "auth_rate_windows" }
