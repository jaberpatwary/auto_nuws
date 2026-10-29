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

// sourcePrioritySQL ranks sources so that:
//   1 = Prothom Alo (সর্বোচ্চ)
//   2 = The Daily Star বাংলা
//   3 = অন্যান্য জাতীয় দৈনিক (ইত্তেফাক, সমকাল, যুগান্তর…)
//   4 = টিভি/আন্তর্জাতিক/প্রযুক্তি মিডিয়া
//   5 = ডেইলি বাংলাদেশ (সর্বনিম্ন)
// একই ৩০ মিনিটের উইন্ডোর মধ্যে এই priority অনুযায়ী সাজানো হবে।
const sourcePrioritySQL = `
	CASE
		WHEN source_name = 'Prothom Alo'            THEN 1
		WHEN source_name = 'দ্য ডেইলি স্টার বাংলা' THEN 2
		WHEN source_name IN (
			'দৈনিক ইত্তেফাক',
			'দৈনিক সমকাল',
			'দৈনিক যুগান্তর',
			'দৈনিক মানবজমিন',
			'দেশ রূপান্তর',
			'বিডিনিউজ২৪ বাংলা',
			'ঢাকা পোস্ট',
			'বাংলা ট্রিবিউন'
		) THEN 3
		WHEN source_name IN (
			'চ্যানেল আই অনলাইন',
			'রাইজিংবিডি',
			'এনটিভি অনলাইন',
			'BBC News বাংলা',
			'ডয়চে ভেলে বাংলা',
			'টেকশহর'
		) THEN 4
		WHEN source_name = 'ডেইলি বাংলাদেশ'        THEN 5
		ELSE 4
	END
`

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

	// Sort by: time bucket (30-min windows) DESC, then source priority, then exact time DESC
	orderSQL := `date_trunc('hour', published_at) + INTERVAL '30 min' * FLOOR(EXTRACT(MINUTE FROM published_at) / 30) DESC, ` +
		sourcePrioritySQL + `, published_at DESC`

	err := q.Order(orderSQL).Limit(limit).Offset(offset).Find(&articles).Error
	return articles, total, err
}

func (r *articleRepository) Delete(id uuid.UUID) error {
	return r.db.Delete(&domain.Article{}, "id = ?", id).Error
}

func (r *articleRepository) UpdateStatus(id uuid.UUID, status string) error {
	return r.db.Model(&domain.Article{}).Where("id = ?", id).Update("status", status).Error
}
