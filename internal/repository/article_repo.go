package repository

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"bangla-news-aggregator/internal/domain"
)

var ErrDuplicateArticle = errors.New("duplicate article")

// ArticleRepository defines persistence operations for articles.
type ArticleRepository interface {
	Create(a *domain.Article) error
	ExistsBySourceURL(sourceURL string) (bool, error)
	ExistsByTitleAndDate(title string, publishedAt time.Time) (bool, error)
	GetByID(id uuid.UUID) (*domain.Article, error)
	List(limit, offset int, category, status string) ([]domain.Article, int64, error)
	Search(limit, offset int, query, category, status string) ([]domain.Article, int64, error)
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
	result := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(a)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrDuplicateArticle
	}
	return nil
}

func (r *articleRepository) ExistsBySourceURL(sourceURL string) (bool, error) {
	var count int64
	err := r.db.Model(&domain.Article{}).Where("source_url = ?", sourceURL).Count(&count).Error
	return count > 0, err
}

func (r *articleRepository) ExistsByTitleAndDate(title string, publishedAt time.Time) (bool, error) {
	var count int64
	err := r.db.Model(&domain.Article{}).
		Where(`lower(regexp_replace(trim(title), '[[:space:][:punct:]]+', ' ', 'g')) = lower(regexp_replace(trim(?), '[[:space:][:punct:]]+', ' ', 'g'))`, title).
		Where(`(published_at AT TIME ZONE 'Asia/Dhaka')::date = (?::timestamptz AT TIME ZONE 'Asia/Dhaka')::date`, publishedAt).
		Count(&count).Error
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

	err := q.Select(`articles.*, ROW_NUMBER() OVER (PARTITION BY source_name ORDER BY published_at DESC, created_at DESC) AS source_rank`).
		Order(`source_rank ASC, published_at DESC, created_at DESC`).
		Limit(limit).Offset(offset).Find(&articles).Error
	return articles, total, err
}

func (r *articleRepository) Search(limit, offset int, query, category, status string) ([]domain.Article, int64, error) {
	var articles []domain.Article
	var total int64
	pattern := "%" + query + "%"
	q := r.db.Model(&domain.Article{}).Where(
		"(title ILIKE ? OR summary ILIKE ? OR content ILIKE ?)", pattern, pattern, pattern,
	)
	if category != "" {
		q = q.Where("category = ?", category)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Select(`articles.*, ROW_NUMBER() OVER (PARTITION BY source_name ORDER BY published_at DESC, created_at DESC) AS source_rank`).
		Order(`source_rank ASC, published_at DESC, created_at DESC`).
		Limit(limit).Offset(offset).Find(&articles).Error
	return articles, total, err
}

func (r *articleRepository) Delete(id uuid.UUID) error {
	return r.db.Delete(&domain.Article{}, "id = ?", id).Error
}

func (r *articleRepository) UpdateStatus(id uuid.UUID, status string) error {
	return r.db.Model(&domain.Article{}).Where("id = ?", id).Update("status", status).Error
}
