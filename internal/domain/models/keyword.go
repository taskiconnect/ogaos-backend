package models

import "github.com/google/uuid"

// Keyword is a single normalised tag (trimmed and title-cased).
type Keyword struct {
	ID   int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Name string `gorm:"uniqueIndex;size:80;not null" json:"name"`
}

// BusinessKeyword is the junction row linking a business to a keyword.
type BusinessKeyword struct {
	BusinessID uuid.UUID `gorm:"type:uuid;primaryKey" json:"business_id"`
	KeywordID  int64     `gorm:"primaryKey"           json:"keyword_id"`

	// Associations (loaded on demand)
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
	Keyword  Keyword  `gorm:"foreignKey:KeywordID"  json:"keyword,omitempty"`
}
