package location

import (
	"strings"

	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// GetStates returns all unique state names from local_government_centers.
func (s *Service) GetStates() []string {
	type row struct {
		State string `gorm:"column:state"`
	}

	var rows []row
	err := s.db.
		Model(&models.LocalGovernmentCenter{}).
		Select("DISTINCT state").
		Order("state ASC").
		Scan(&rows).Error
	if err != nil {
		return []string{}
	}

	out := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))

	for _, r := range rows {
		state := strings.TrimSpace(r.State)
		if state == "" {
			continue
		}

		key := strings.ToLower(state)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, state)
	}

	return out
}

// GetLGAs returns all LGAs for a given state name (case-insensitive).
// The second return value is false when the state is not found.
func (s *Service) GetLGAs(state string) ([]string, bool) {
	state = strings.TrimSpace(state)
	if state == "" {
		return nil, false
	}

	type row struct {
		LocalGovernment string `gorm:"column:local_government"`
	}

	var rows []row
	err := s.db.
		Model(&models.LocalGovernmentCenter{}).
		Select("DISTINCT local_government").
		Where("LOWER(TRIM(state)) = LOWER(TRIM(?))", state).
		Order("local_government ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, false
	}

	if len(rows) == 0 {
		return nil, false
	}

	out := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))

	for _, r := range rows {
		lga := strings.TrimSpace(r.LocalGovernment)
		if lga == "" {
			continue
		}

		key := strings.ToLower(lga)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, lga)
	}

	if len(out) == 0 {
		return nil, false
	}

	return out, true
}
