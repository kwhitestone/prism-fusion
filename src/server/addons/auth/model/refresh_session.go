package model

import "time"

// RefreshSession stores only a SHA-256 token hash. The browser receives the
// random opaque token once; database disclosure alone cannot recover it.
type RefreshSession struct {
	ID                  uint       `json:"id" gorm:"primarykey"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
	UserID              uint       `json:"userId" gorm:"index;not null"`
	FamilyID            string     `json:"-" gorm:"size:36;index;not null"`
	TokenHash           string     `json:"-" gorm:"size:64;uniqueIndex;not null"`
	ExpiresAt           time.Time  `json:"expiresAt" gorm:"index;not null"`
	FamilyExpiresAt     time.Time  `json:"familyExpiresAt" gorm:"index;not null"`
	RevokedAt           *time.Time `json:"-" gorm:"index"`
	ReplacedByHash      string     `json:"-" gorm:"size:64"`
	RotationRequestHash string     `json:"-" gorm:"size:64"`
}
