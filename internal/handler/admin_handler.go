package handler

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"bangla-news-aggregator/internal/cron"
	"bangla-news-aggregator/internal/usecase"
)

// AdminHandler serves the protected back-office UI.
type AdminHandler struct {
	authUC    usecase.AuthUsecase
	articleUC usecase.ArticleUsecase
	feedUC    usecase.FeedUsecase
	worker    *cron.Worker
	cookieTTL time.Duration
}

func NewAdminHandler(authUC usecase.AuthUsecase, articleUC usecase.ArticleUsecase, feedUC usecase.FeedUsecase, worker *cron.Worker, cookieTTL time.Duration) *AdminHandler {
	if cookieTTL <= 0 {
		cookieTTL = 24 * time.Hour
	}
	return &AdminHandler{authUC: authUC, articleUC: articleUC, feedUC: feedUC, worker: worker, cookieTTL: cookieTTL}
}

// LoginPage renders the login form.
func (h *AdminHandler) LoginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", gin.H{})
}

// Login verifies credentials and sets the admin_token cookie.
func (h *AdminHandler) Login(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	token, err := h.authUC.Login(username, password)
	if err != nil {
		c.Set("login_failed", true)
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{"Error": "ভুল ইউজারনেম বা পাসওয়ার্ড"})
		return
	}

	secure := os.Getenv("APP_ENV") == "production"
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(cookieName, token, int(h.cookieTTL.Seconds()), "/", "", secure, true)
	c.Redirect(http.StatusFound, "/admin/dashboard")
}

// Logout clears the admin cookie.
func (h *AdminHandler) Logout(c *gin.Context) {
	secure := os.Getenv("APP_ENV") == "production"
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(cookieName, "", -1, "/", "", secure, true)
	c.Redirect(http.StatusFound, "/admin/login")
}

// Dashboard shows summary stats and recent articles.
func (h *AdminHandler) Dashboard(c *gin.Context) {
	articles, total, _ := h.articleUC.List(1, 10, "", "")
	feeds, _ := h.feedUC.List()

	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"Articles":   articles,
		"TotalCount": total,
		"Feeds":      feeds,
		"Username":   c.GetString("admin_username"),
		"CSRFToken":  c.GetString(csrfContextKey),
	})
}

// ArticlesList shows all articles with management actions.
func (h *AdminHandler) ArticlesList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	status := c.Query("status")

	articles, total, err := h.articleUC.List(page, 20, "", status)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"message": "লোড ব্যর্থ হয়েছে"})
		return
	}

	c.HTML(http.StatusOK, "articles.html", gin.H{
		"Articles":  articles,
		"Total":     total,
		"Page":      page,
		"Status":    status,
		"CSRFToken": c.GetString(csrfContextKey),
	})
}

// DeleteArticle removes an article by ID.
func (h *AdminHandler) DeleteArticle(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.articleUC.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Redirect(http.StatusFound, "/admin/articles")
}

// UpdateArticleStatus publishes/archives an article.
func (h *AdminHandler) UpdateArticleStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	status := c.PostForm("status")
	if err := h.articleUC.UpdateStatus(id, status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Redirect(http.StatusFound, "/admin/articles")
}

// FeedsList shows all configured RSS feeds.
func (h *AdminHandler) FeedsList(c *gin.Context) {
	feeds, err := h.feedUC.List()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"message": "লোড ব্যর্থ হয়েছে"})
		return
	}
	c.HTML(http.StatusOK, "feeds.html", gin.H{
		"Feeds":     feeds,
		"CSRFToken": c.GetString(csrfContextKey),
	})
}

// CreateFeed adds a new RSS feed source.
func (h *AdminHandler) CreateFeed(c *gin.Context) {
	name := c.PostForm("name")
	url := c.PostForm("url")
	category := c.PostForm("category")

	if name == "" || url == "" {
		c.Redirect(http.StatusFound, "/admin/feeds")
		return
	}

	if _, err := h.feedUC.Create(name, url, category); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Redirect(http.StatusFound, "/admin/feeds")
}

// DeleteFeed removes a feed source.
func (h *AdminHandler) DeleteFeed(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.feedUC.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Redirect(http.StatusFound, "/admin/feeds")
}

// ToggleFeed activates/deactivates a feed source.
func (h *AdminHandler) ToggleFeed(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	active := c.PostForm("active") == "true"
	if err := h.feedUC.SetActive(id, active); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Redirect(http.StatusFound, "/admin/feeds")
}

// RunCronNow triggers an immediate, synchronous pipeline run (useful for
// testing from the admin UI without waiting for the next hourly tick).
func (h *AdminHandler) RunCronNow(c *gin.Context) {
	// Use a background context, not the request's: the request context is
	// cancelled as soon as this handler returns, which would kill the run.
	go h.worker.RunOnce(context.Background())
	c.Redirect(http.StatusFound, "/admin/dashboard")
}

// PasswordPage renders the signed-in administrator's password form.
func (h *AdminHandler) PasswordPage(c *gin.Context) {
	c.HTML(http.StatusOK, "password.html", gin.H{
		"Username":  c.GetString("admin_username"),
		"CSRFToken": c.GetString(csrfContextKey),
	})
}

// ChangePassword verifies the current password and stores a bcrypt hash.
func (h *AdminHandler) ChangePassword(c *gin.Context) {
	current := c.PostForm("current_password")
	newPassword := c.PostForm("new_password")
	confirm := c.PostForm("confirm_password")
	csrfTok := c.GetString(csrfContextKey)
	if newPassword != confirm {
		c.HTML(http.StatusBadRequest, "password.html", gin.H{"Error": "নতুন পাসওয়ার্ড দুটি মেলেনি", "Username": c.GetString("admin_username"), "CSRFToken": csrfTok})
		return
	}
	if err := h.authUC.ChangePassword(c.GetString("admin_username"), current, newPassword); err != nil {
		message := "পাসওয়ার্ড পরিবর্তন করা যায়নি। বর্তমান পাসওয়ার্ড যাচাই করুন এবং নতুন পাসওয়ার্ড অন্তত ১২ অক্ষরের দিন।"
		c.HTML(http.StatusBadRequest, "password.html", gin.H{"Error": message, "Username": c.GetString("admin_username"), "CSRFToken": csrfTok})
		return
	}
	c.HTML(http.StatusOK, "password.html", gin.H{"Success": "পাসওয়ার্ড সফলভাবে পরিবর্তন হয়েছে।", "Username": c.GetString("admin_username"), "CSRFToken": csrfTok})
}
