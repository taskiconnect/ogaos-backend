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

const (
	maxKeywordsPerBusiness = 15
	maxKeywordSuggestions  = 10
)

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
		Joins("JOIN keywords ON keywords.id = business_keywords.keyword_id").
		Where("business_keywords.business_id = ?", businessID).
		Order("keywords.name ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	names := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.Keyword.Name != "" {
			names = append(names, r.Keyword.Name)
		}
	}

	return names, nil
}

// GetKeywordsBySlug returns the keyword names for a business identified by its slug.
func (s *Service) GetKeywordsBySlug(slug string) ([]string, error) {
	var biz models.Business
	if err := s.db.Where("slug = ?", slug).First(&biz).Error; err != nil {
		return nil, err
	}

	return s.GetKeywords(biz.ID)
}

// SuggestKeywords returns existing keywords that match a user's query.
// Used to power autocomplete while typing.
func (s *Service) SuggestKeywords(query string) ([]string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []string{}, nil
	}

	normalisedQuery := normaliseKeyword(query)
	if normalisedQuery == "" {
		return []string{}, nil
	}

	like := normalisedQuery + "%"

	var rows []models.Keyword
	if err := s.db.
		Where("name ILIKE ?", like).
		Order("name ASC").
		Limit(maxKeywordSuggestions).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Name != "" {
			out = append(out, row.Name)
		}
	}

	return out, nil
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
		keywordIDs := make([]int64, 0, len(normalised))

		for _, name := range normalised {
			// Insert if missing, otherwise reuse existing row.
			if err := tx.
				Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "name"}},
					DoNothing: true,
				}).
				Create(&models.Keyword{Name: name}).Error; err != nil {
				return err
			}

			var kw models.Keyword
			if err := tx.Where("name = ?", name).First(&kw).Error; err != nil {
				return err
			}

			keywordIDs = append(keywordIDs, kw.ID)
		}

		// Remove all existing keyword links for this business.
		if err := tx.
			Where("business_id = ?", businessID).
			Delete(&models.BusinessKeyword{}).Error; err != nil {
			return err
		}

		// Insert fresh junction rows.
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

		// Deduplicate case-insensitively.
		key := strings.ToLower(n)
		if _, exists := seen[key]; exists {
			continue
		}

		seen[key] = struct{}{}
		out = append(out, n)
	}

	return out
}

func normaliseKeyword(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	parts := strings.Fields(s)
	for i, part := range parts {
		parts[i] = titleWord(strings.ToLower(part))
	}

	return strings.Join(parts, " ")
}

func titleWord(s string) string {
	if s == "" {
		return s
	}

	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])

	for i := 1; i < len(runes); i++ {
		runes[i] = unicode.ToLower(runes[i])
	}

	return string(runes)
}
