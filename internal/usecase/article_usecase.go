package usecase

import (
	"errors"
	"github.com/google/uuid"
	"strings"

	"bangla-news-aggregator/internal/domain"
	"bangla-news-aggregator/internal/repository"
)

type ArticleUsecase interface {
	List(page, pageSize int, category, status string) ([]domain.Article, int64, error)
	Search(page, pageSize int, query, category, status string) ([]domain.Article, int64, error)
	GetByID(id uuid.UUID) (*domain.Article, error)
	Delete(id uuid.UUID) error
	UpdateStatus(id uuid.UUID, status string) error
}

type articleUsecase struct {
	repo repository.ArticleRepository
}

func NewArticleUsecase(repo repository.ArticleRepository) ArticleUsecase {
	return &articleUsecase{repo: repo}
}

func (u *articleUsecase) List(page, pageSize int, category, status string) ([]domain.Article, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	return u.repo.List(pageSize, offset, category, status)
}

func (u *articleUsecase) Search(page, pageSize int, query, category, status string) ([]domain.Article, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	query = strings.TrimSpace(query)
	if len([]rune(query)) > 100 {
		query = string([]rune(query)[:100])
	}
	if query == "" {
		return u.List(page, pageSize, category, status)
	}
	return u.repo.Search(pageSize, (page-1)*pageSize, query, category, status)
}

func (u *articleUsecase) GetByID(id uuid.UUID) (*domain.Article, error) {
	return u.repo.GetByID(id)
}

func (u *articleUsecase) Delete(id uuid.UUID) error {
	return u.repo.Delete(id)
}

func (u *articleUsecase) UpdateStatus(id uuid.UUID, status string) error {
	if status != "draft" && status != "published" && status != "archived" {
		return errors.New("status must be draft, published, or archived")
	}
	return u.repo.UpdateStatus(id, status)
}
