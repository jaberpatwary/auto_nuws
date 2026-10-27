package repository

import (
	"context"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"bangla-news-aggregator/internal/config"
	"bangla-news-aggregator/internal/domain"
)

// NewPostgresDB opens a GORM connection to Postgres (Neon/Supabase or local),
// runs auto-migration for all domain models, and seeds a default admin user
// plus a starter set of Bangladeshi RSS feeds if none exist yet.
func NewPostgresDB(cfg *config.Config) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(
		&domain.Article{},
		&domain.FeedSource{},
		&domain.AIUsage{},
		&domain.AdminUser{},
	); err != nil {
		return nil, err
	}

	seedAdmin(db, cfg)
	seedFeeds(db)

	return db, nil
}

func seedAdmin(db *gorm.DB, cfg *config.Config) {
	var count int64
	db.Model(&domain.AdminUser{}).Count(&count)
	if count > 0 {
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminDefaultPass), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("[seed] could not hash default admin password: %v", err)
		return
	}

	admin := domain.AdminUser{
		Username:     cfg.AdminDefaultUser,
		PasswordHash: string(hash),
	}
	if err := db.Create(&admin).Error; err != nil {
		log.Printf("[seed] could not create default admin user: %v", err)
		return
	}
	log.Printf("[seed] created default admin user %q — change this password immediately", cfg.AdminDefaultUser)
}

func seedFeeds(db *gorm.DB) {
	var count int64
	db.Model(&domain.FeedSource{}).Count(&count)
	if count > 0 {
		return
	}

	defaults := []domain.FeedSource{
		{Name: "Prothom Alo", URL: "https://www.prothomalo.com/feed", Category: "general", Active: true},
		{Name: "The Daily Star বাংলা", URL: "https://news.google.com/rss/search?q=site:bangla.thedailystar.net&hl=bn&gl=BD&ceid=BD:bn", Category: "general", Active: true},
		{Name: "Google News বাংলা (Bangladesh)", URL: "https://news.google.com/rss?hl=bn&gl=BD&ceid=BD:bn", Category: "general", Active: true},
		{Name: "BBC News বাংলা", URL: "https://feeds.bbci.co.uk/bengali/rss.xml", Category: "international", Active: true},
	}

	if err := db.Create(&defaults).Error; err != nil {
		log.Printf("[seed] could not seed default feed sources: %v", err)
		return
	}
	log.Printf("[seed] seeded %d default RSS feed sources — verify their real feed URLs", len(defaults))
}
