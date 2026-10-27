package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Article represents a single news article, either AI-summarized from an
// RSS feed or manually created by an admin.
type Article struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Title       string    `gorm:"type:text;not null" json:"title"`
	Content     string    `gorm:"type:text;not null" json:"content"`
	Summary     string    `gorm:"type:text" json:"summary"`
	SourceURL   string    `gorm:"type:text;uniqueIndex;not null" json:"source_url"`
	SourceName  string    `gorm:"type:varchar(255)" json:"source_name"`
	ImageURL    string    `gorm:"type:text" json:"image_url"`
	Category    string    `gorm:"type:varchar(100);index" json:"category"`
	Status      string    `gorm:"type:varchar(20);default:'published';index" json:"status"` // draft | published | archived
	PublishedAt time.Time `gorm:"index" json:"published_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// BeforeCreate ensures every article has a UUID even on drivers where the
// default gen_random_uuid() extension might not be enabled.
func (a *Article) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.PublishedAt.IsZero() {
		a.PublishedAt = time.Now()
	}
	return nil
}

// FeedSource represents one RSS feed the cron worker pulls from.
type FeedSource struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name      string    `gorm:"type:varchar(255);not null" json:"name"`
	URL       string    `gorm:"type:text;uniqueIndex;not null" json:"url"`
	Category  string    `gorm:"type:varchar(100)" json:"category"`
	Active    bool      `gorm:"default:true" json:"active"`
	LastFetch time.Time `json:"last_fetch"`
	LastError string    `gorm:"type:text" json:"last_error"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (f *FeedSource) BeforeCreate(tx *gorm.DB) error {
	if f.ID == uuid.Nil {
		f.ID = uuid.New()
	}
	return nil
}

// AIUsage logs every call made to Groq/Gemini for cost & reliability tracking.
type AIUsage struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Provider   string     `gorm:"type:varchar(50);not null" json:"provider"` // groq | gemini
	Model      string     `gorm:"type:varchar(100)" json:"model"`
	ArticleID  *uuid.UUID `gorm:"type:uuid;index" json:"article_id"`
	TokensUsed int        `json:"tokens_used"`
	Success    bool       `json:"success"`
	ErrorMsg   string     `gorm:"type:text" json:"error_msg"`
	DurationMs int64      `json:"duration_ms"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (u *AIUsage) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}

// AdminUser represents a back-office user who can log into /admin.
type AdminUser struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Username     string    `gorm:"type:varchar(100);uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"type:text;not null" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (u *AdminUser) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}
