// internal/service/store/service.go
package store

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

type CreateRequest struct {
	Name        string  `json:"name" binding:"required"`
	Description *string `json:"description"`
	Street      *string `json:"street"`
	CityTown    *string `json:"city_town"`
	State       *string `json:"state"`
	Phone       *string `json:"phone"`
	IsDefault   bool    `json:"is_default"`
}

type UpdateRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Street      *string `json:"street"`
	CityTown    *string `json:"city_town"`
	State       *string `json:"state"`
	Phone       *string `json:"phone"`
}

func (s *Service) Create(businessID uuid.UUID, req CreateRequest) (*models.Store, error) {
	store := models.Store{
		BusinessID:  businessID,
		Name:        req.Name,
		Description: req.Description,
		Street:      req.Street,
		CityTown:    req.CityTown,
		State:       req.State,
		Phone:       req.Phone,
		IsDefault:   req.IsDefault,
	}

	return &store, s.db.Transaction(func(tx *gorm.DB) error {
		// If this is set as default, unset any existing default
		if req.IsDefault {
			tx.Model(&models.Store{}).Where("business_id = ? AND is_default = true", businessID).
				Update("is_default", false)
		}
		return tx.Create(&store).Error
	})
}

func (s *Service) Get(businessID, storeID uuid.UUID) (*models.Store, error) {
	var store models.Store
	if err := s.db.Where("id = ? AND business_id = ? AND is_active = true", storeID, businessID).First(&store).Error; err != nil {
		return nil, errors.New("store not found")
	}
	return &store, nil
}

func (s *Service) List(businessID uuid.UUID) ([]models.Store, error) {
	var stores []models.Store
	err := s.db.Where("business_id = ? AND is_active = true", businessID).
		Order("is_default DESC, name ASC").Find(&stores).Error
	return stores, err
}

func (s *Service) Update(businessID, storeID uuid.UUID, req UpdateRequest) (*models.Store, error) {
	var store models.Store
	if err := s.db.Where("id = ? AND business_id = ?", storeID, businessID).First(&store).Error; err != nil {
		return nil, errors.New("store not found")
	}
	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Street != nil {
		updates["street"] = *req.Street
	}
	if req.CityTown != nil {
		updates["city_town"] = *req.CityTown
	}
	if req.State != nil {
		updates["state"] = *req.State
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if err := s.db.Model(&store).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &store, nil
}

func (s *Service) SetDefault(businessID, storeID uuid.UUID) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		tx.Model(&models.Store{}).Where("business_id = ?", businessID).Update("is_default", false)
		result := tx.Model(&models.Store{}).
			Where("id = ? AND business_id = ?", storeID, businessID).
			Update("is_default", true)
		if result.RowsAffected == 0 {
			return errors.New("store not found")
		}
		return result.Error
	})
}

func (s *Service) Delete(businessID, storeID uuid.UUID) error {
	var store models.Store
	if err := s.db.Where("id = ? AND business_id = ?", storeID, businessID).First(&store).Error; err != nil {
		return errors.New("store not found")
	}
	if store.IsDefault {
		return errors.New("cannot delete the default store")
	}
	return s.db.Model(&store).Update("is_active", false).Error
}
