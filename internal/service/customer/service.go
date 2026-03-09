// internal/service/customer/service.go
package customer

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ogaos-backend/internal/domain/models"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// ─── DTOs ────────────────────────────────────────────────────────────────────

type CreateRequest struct {
	FirstName   string  `json:"first_name" binding:"required"`
	LastName    string  `json:"last_name" binding:"required"`
	Email       *string `json:"email"`
	PhoneNumber *string `json:"phone_number"`
	Address     *string `json:"address"`
	Notes       *string `json:"notes"`
}

type UpdateRequest struct {
	FirstName   *string `json:"first_name"`
	LastName    *string `json:"last_name"`
	Email       *string `json:"email"`
	PhoneNumber *string `json:"phone_number"`
	Address     *string `json:"address"`
	Notes       *string `json:"notes"`
}

type ListFilter struct {
	Search string
	Page   int
	Limit  int
}

// ─── Methods ─────────────────────────────────────────────────────────────────

func (s *Service) Create(businessID uuid.UUID, req CreateRequest) (*models.Customer, error) {
	c := models.Customer{
		BusinessID:  businessID,
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		Email:       req.Email,
		PhoneNumber: req.PhoneNumber,
		Address:     req.Address,
		Notes:       req.Notes,
	}
	if err := s.db.Create(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) Get(businessID, customerID uuid.UUID) (*models.Customer, error) {
	var c models.Customer
	if err := s.db.Where("id = ? AND business_id = ?", customerID, businessID).First(&c).Error; err != nil {
		return nil, errors.New("customer not found")
	}
	return &c, nil
}

func (s *Service) List(businessID uuid.UUID, filter ListFilter) ([]models.Customer, int64, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 20
	}
	offset := (filter.Page - 1) * filter.Limit

	q := s.db.Model(&models.Customer{}).Where("business_id = ? AND is_active = true", businessID)
	if filter.Search != "" {
		like := "%" + filter.Search + "%"
		q = q.Where("first_name ILIKE ? OR last_name ILIKE ? OR email ILIKE ? OR phone_number ILIKE ?", like, like, like, like)
	}

	var total int64
	q.Count(&total)

	var customers []models.Customer
	if err := q.Offset(offset).Limit(filter.Limit).Order("created_at DESC").Find(&customers).Error; err != nil {
		return nil, 0, err
	}
	return customers, total, nil
}

func (s *Service) Update(businessID, customerID uuid.UUID, req UpdateRequest) (*models.Customer, error) {
	var c models.Customer
	if err := s.db.Where("id = ? AND business_id = ?", customerID, businessID).First(&c).Error; err != nil {
		return nil, errors.New("customer not found")
	}

	updates := map[string]interface{}{}
	if req.FirstName != nil {
		updates["first_name"] = *req.FirstName
	}
	if req.LastName != nil {
		updates["last_name"] = *req.LastName
	}
	if req.Email != nil {
		updates["email"] = *req.Email
	}
	if req.PhoneNumber != nil {
		updates["phone_number"] = *req.PhoneNumber
	}
	if req.Address != nil {
		updates["address"] = *req.Address
	}
	if req.Notes != nil {
		updates["notes"] = *req.Notes
	}

	if err := s.db.Model(&c).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) Delete(businessID, customerID uuid.UUID) error {
	result := s.db.Model(&models.Customer{}).
		Where("id = ? AND business_id = ?", customerID, businessID).
		Update("is_active", false)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("customer not found")
	}
	return nil
}
