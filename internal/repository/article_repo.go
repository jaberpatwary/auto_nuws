package repository

import (
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"bangla-news-aggregator/internal/domain"
)

// ArticleRepository defines persistence operations for articles.
type ArticleRepository interface {
	Create(a *domain.Article) error
	ExistsBySourceURL(sourceURL string) (bool, error)
	GetByID(id uuid.UUID) (*domain.Article, error)
	List(limit, offset int, category, status string) ([]domain.Article, int64, error)
	Delete(id uuid.UUID) error
	UpdateStatus(id uuid.UUID, status string) error
}

type articleRepository struct {
	db *gorm.DB
}

func NewArticleRepository(db *gorm.DB) ArticleRepository {
	return &articleRepository{db: db}
}

func (r *articleRepository) Create(a *domain.Article) error {
	return r.db.Create(a).Error
}

func (r *articleRepository) ExistsBySourceURL(sourceURL string) (bool, error) {
	var count int64
	err := r.db.Model(&domain.Article{}).Where("source_url = ?", sourceURL).Count(&count).Error
	return count > 0, err
}

func (r *articleRepository) GetByID(id uuid.UUID) (*domain.Article, error) {
	var a domain.Article
	err := r.db.First(&a, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &a, err
}

func (r *articleRepository) List(limit, offset int, category, status string) ([]domain.Article, int64, error) {
	var articles []domain.Article
	var total int64

	q := r.db.Model(&domain.Article{})
	if category != "" {
		q = q.Where("category = ?", category)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}

	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := q.Order("published_at DESC").Limit(limit).Offset(offset).Find(&articles).Error
	return articles, total, err
}

func (r *articleRepository) Delete(id uuid.UUID) error {
	return r.db.Delete(&domain.Article{}, "id = ?", id).Error
}

func (r *articleRepository) UpdateStatus(id uuid.UUID, status string) error {
	return r.db.Model(&domain.Article{}).Where("id = ?", id).Update("status", status).Error
}
