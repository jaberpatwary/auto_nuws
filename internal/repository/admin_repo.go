package repository

import (
	"errors"

	"gorm.io/gorm"

	"bangla-news-aggregator/internal/domain"
)

// AdminRepository defines persistence operations for admin users.
type AdminRepository interface {
	GetByUsername(username string) (*domain.AdminUser, error)
	UpdatePassword(username, passwordHash string) error
}

type adminRepository struct {
	db *gorm.DB
}

func NewAdminRepository(db *gorm.DB) AdminRepository {
	return &adminRepository{db: db}
}

func (r *adminRepository) GetByUsername(username string) (*domain.AdminUser, error) {
	var u domain.AdminUser
	err := r.db.First(&u, "username = ?", username).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &u, err
}

func (r *adminRepository) UpdatePassword(username, passwordHash string) error {
	return r.db.Model(&domain.AdminUser{}).Where("username = ?", username).Update("password_hash", passwordHash).Error
}

// AIUsageRepository logs AI provider calls for cost & reliability tracking.
type AIUsageRepository interface {
	Create(u *domain.AIUsage) error
	Stats(limit int) ([]domain.AIUsage, error)
}

type aiUsageRepository struct {
	db *gorm.DB
}

func NewAIUsageRepository(db *gorm.DB) AIUsageRepository {
	return &aiUsageRepository{db: db}
}

func (r *aiUsageRepository) Create(u *domain.AIUsage) error {
	return r.db.Create(u).Error
}

func (r *aiUsageRepository) Stats(limit int) ([]domain.AIUsage, error) {
	var usages []domain.AIUsage
	err := r.db.Order("created_at DESC").Limit(limit).Find(&usages).Error
	return usages, err
}
