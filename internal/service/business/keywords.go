// internal/service/business/keywords.go
package business

import (
	"errors"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ogaos-backend/internal/domain/models"
)

const maxKeywordsPerBusiness = 15

// ─── DTOs ─────────────────────────────────────────────────────────────────────

type SetKeywordsRequest struct {
	Keywords []string `json:"keywords"`
}

// ─── Public API ───────────────────────────────────────────────────────────────

// GetKeywords returns the current keyword list for a business (names only, sorted alphabetically).
func (s *Service) GetKeywords(businessID uuid.UUID) ([]string, error) {
	var rows []models.BusinessKeyword
	if err := s.db.
		Preload("Keyword").
		Where("business_id = ?", businessID).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.Keyword.Name)
	}
	return names, nil
}

// GetKeywordsBySlug returns the keyword names for a business identified by its slug.
// Used by the public endpoint: GET /public/business/:slug/keywords
func (s *Service) GetKeywordsBySlug(slug string) ([]string, error) {
	var biz models.Business
	if err := s.db.Where("slug = ?", slug).First(&biz).Error; err != nil {
		return nil, err
	}

	return s.GetKeywords(biz.ID)
}

// SetKeywords replaces a business's entire keyword set with the supplied list.
// It is idempotent and safe to call repeatedly.
func (s *Service) SetKeywords(businessID uuid.UUID, req SetKeywordsRequest) ([]string, error) {
	normalised := normaliseKeywords(req.Keywords)
	if len(normalised) > maxKeywordsPerBusiness {
		return nil, errors.New("maximum 15 keywords allowed per business")
	}

	var result []string

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// 1. Upsert keywords and collect their IDs
		keywordIDs := make([]int64, 0, len(normalised))
		for _, name := range normalised {
			kw := models.Keyword{Name: name}
			if err := tx.
				Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "name"}},
					DoUpdates: clause.AssignmentColumns([]string{"name"}),
				}).
				FirstOrCreate(&kw, models.Keyword{Name: name}).Error; err != nil {
				return err
			}
			keywordIDs = append(keywordIDs, kw.ID)
		}

		// 2. Remove all existing junction rows for this business
		if err := tx.
			Where("business_id = ?", businessID).
			Delete(&models.BusinessKeyword{}).Error; err != nil {
			return err
		}

		// 3. Insert fresh junction rows
		junctions := make([]models.BusinessKeyword, 0, len(keywordIDs))
		for _, kid := range keywordIDs {
			junctions = append(junctions, models.BusinessKeyword{
				BusinessID: businessID,
				KeywordID:  kid,
			})
		}
		if len(junctions) > 0 {
			if err := tx.Create(&junctions).Error; err != nil {
				return err
			}
		}

		result = normalised
		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func normaliseKeywords(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		n := normaliseKeyword(s)
		if n == "" || len(n) > 80 {
			continue
		}
		if _, exists := seen[n]; exists {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

func normaliseKeyword(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteRune(' ')
			}
			prevSpace = true
		} else {
			b.WriteRune(unicode.ToLower(r))
			prevSpace = false
		}
	}
	return b.String()
}
