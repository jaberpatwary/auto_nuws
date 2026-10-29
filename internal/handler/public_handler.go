package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"bangla-news-aggregator/internal/usecase"
)

// PublicHandler serves the reader-facing pages: homepage, category feeds,
// and individual article pages.
type PublicHandler struct {
	articleUC usecase.ArticleUsecase
}

func NewPublicHandler(articleUC usecase.ArticleUsecase) *PublicHandler {
	return &PublicHandler{articleUC: articleUC}
}

// Home renders a paginated list of the latest published articles.
func (h *PublicHandler) Home(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	category := c.Query("category")

	articles, total, err := h.articleUC.List(page, 20, category, "published")
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"message": "নিবন্ধ লোড করা যায়নি"})
		return
	}

	c.HTML(http.StatusOK, "index.html", gin.H{
		"Articles": articles,
		"Total":    total,
		"Page":     page,
		"Category": category,
		"HasMore":  int64(page*20) < total,
	})
}

// ArticleDetail renders a single article by ID.
func (h *PublicHandler) ArticleDetail(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"message": "নিবন্ধ খুঁজে পাওয়া যায়নি"})
		return
	}

	article, err := h.articleUC.GetByID(id)
	if err != nil || article == nil || article.Status != "published" {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"message": "নিবন্ধ খুঁজে পাওয়া যায়নি"})
		return
	}

	c.HTML(http.StatusOK, "article.html", gin.H{"Article": article})
}

// HealthCheck is a simple liveness probe for Render.com / uptime monitors.
func (h *PublicHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// LatestCheck returns the ID, title, and count of latest published articles for auto-update checks.
func (h *PublicHandler) LatestCheck(c *gin.Context) {
	category := c.Query("category")
	articles, total, err := h.articleUC.List(1, 1, category, "published")
	if err != nil || len(articles) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"total":        total,
			"latest_id":    "",
			"latest_title": "",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"total":        total,
		"latest_id":    articles[0].ID.String(),
		"latest_title": articles[0].Title,
		"published_at": articles[0].PublishedAt,
	})
}
