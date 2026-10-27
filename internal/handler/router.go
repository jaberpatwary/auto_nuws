package handler

import (
	"fmt"
	"html"
	"html/template"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"bangla-news-aggregator/internal/usecase"
)

var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

func stripHTML(input string) string {
	cleaned := htmlTagRegex.ReplaceAllString(input, " ")
	if idx := strings.LastIndex(cleaned, "<"); idx != -1 {
		if !strings.Contains(cleaned[idx:], ">") {
			cleaned = cleaned[:idx]
		}
	}
	cleaned = html.UnescapeString(cleaned)
	return strings.TrimSpace(strings.Join(strings.Fields(cleaned), " "))
}

func toBanglaDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r - '0' + '০')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func bangla12HourTime(t time.Time) string {
	loc, err := time.LoadLocation("Asia/Dhaka")
	if err == nil {
		t = t.In(loc)
	}
	hour := t.Hour()
	minute := t.Minute()

	var period string
	switch {
	case hour >= 4 && hour < 6:
		period = "ভোর"
	case hour >= 6 && hour < 12:
		period = "সকাল"
	case hour >= 12 && hour < 15:
		period = "দুপুর"
	case hour >= 15 && hour < 18:
		period = "বিকাল"
	case hour >= 18 && hour < 20:
		period = "সন্ধ্যা"
	default:
		period = "রাত"
	}

	h12 := hour % 12
	if h12 == 0 {
		h12 = 12
	}

	return fmt.Sprintf("%s %s:%s", period, toBanglaDigits(fmt.Sprintf("%02d", h12)), toBanglaDigits(fmt.Sprintf("%02d", minute)))
}

func banglaDateTime(t time.Time) string {
	loc, err := time.LoadLocation("Asia/Dhaka")
	if err == nil {
		t = t.In(loc)
	}
	bengaliMonths := []string{
		"", "জানুয়ারি", "ফেব্রুয়ারি", "মার্চ", "এপ্রিল", "মে", "জুন",
		"জুলাই", "আগস্ট", "সেপ্টেম্বর", "অক্টোবর", "নভেম্বর", "ডিসেম্বর",
	}
	monthName := bengaliMonths[t.Month()]
	return fmt.Sprintf("%s %s %s, %s", toBanglaDigits(t.Format("02")), monthName, toBanglaDigits(t.Format("2006")), bangla12HourTime(t))
}

func banglaFullDateTime(t time.Time) string {
	loc, err := time.LoadLocation("Asia/Dhaka")
	if err == nil {
		t = t.In(loc)
	}
	bengaliDays := []string{
		"রবিবার", "সোমবার", "মঙ্গলবার", "বুধবার", "বৃহস্পতিবার", "শুক্রবার", "শনিবার",
	}
	bengaliMonths := []string{
		"", "জানুয়ারি", "ফেব্রুয়ারি", "মার্চ", "এপ্রিল", "মে", "জুন",
		"জুলাই", "আগস্ট", "সেপ্টেম্বর", "অক্টোবর", "নভেম্বর", "ডিসেম্বর",
	}
	dayName := bengaliDays[t.Weekday()]
	monthName := bengaliMonths[t.Month()]
	return fmt.Sprintf("%s, %s %s %s, %s", dayName, toBanglaDigits(t.Format("02")), monthName, toBanglaDigits(t.Format("2006")), bangla12HourTime(t))
}

func banglaTimeAgo(t time.Time) string {
	loc, err := time.LoadLocation("Asia/Dhaka")
	if err == nil {
		t = t.In(loc)
	}
	now := time.Now().In(t.Location())
	diff := now.Sub(t)
	if diff < 0 {
		diff = 0
	}

	minutes := int(diff.Minutes())
	hours := int(diff.Hours())
	days := hours / 24

	if minutes < 1 {
		return "এইমাত্র"
	} else if minutes < 60 {
		return fmt.Sprintf("%s মিনিট আগে", toBanglaDigits(strconv.Itoa(minutes)))
	} else if hours < 24 {
		return fmt.Sprintf("%s ঘণ্টা আগে", toBanglaDigits(strconv.Itoa(hours)))
	} else if days < 7 {
		return fmt.Sprintf("%s দিন আগে", toBanglaDigits(strconv.Itoa(days)))
	}
	return banglaDateTime(t)
}

func todayBanglaDate() string {
	loc, err := time.LoadLocation("Asia/Dhaka")
	now := time.Now()
	if err == nil {
		now = now.In(loc)
	}
	bengaliDays := []string{
		"রবিবার", "সোমবার", "মঙ্গলবার", "বুধবার", "বৃহস্পতিবার", "শুক্রবার", "শনিবার",
	}
	bengaliMonths := []string{
		"", "জানুয়ারি", "ফেব্রুয়ারি", "মার্চ", "এপ্রিল", "মে", "জুন",
		"জুলাই", "আগস্ট", "সেপ্টেম্বর", "অক্টোবর", "নভেম্বর", "ডিসেম্বর",
	}
	return fmt.Sprintf("%s, %s %s %s", bengaliDays[now.Weekday()], toBanglaDigits(now.Format("02")), bengaliMonths[now.Month()], toBanglaDigits(now.Format("2006")))
}

// NewRouter builds the full Gin engine: templates, static files, public
// routes, and JWT-protected admin routes.
func NewRouter(
	authUC usecase.AuthUsecase,
	publicH *PublicHandler,
	adminH *AdminHandler,
) *gin.Engine {
	r := gin.Default()
	r.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	})

	r.SetFuncMap(template.FuncMap{
		"add":                func(a, b int) int { return a + b },
		"sub":                func(a, b int) int { return a - b },
		"safeHTML":           func(s string) template.HTML { return template.HTML(s) },
		"stripHTML":          stripHTML,
		"banglaDigits":       toBanglaDigits,
		"banglaDate":         banglaDateTime,
		"banglaDateTime":     banglaDateTime,
		"banglaFullDateTime": banglaFullDateTime,
		"banglaTimeAgo":      banglaTimeAgo,
		"todayBanglaDate":    todayBanglaDate,
	})
	r.LoadHTMLGlob("web/templates/**/*.html")
	r.Static("/static", "web/static")

	r.NoRoute(func(c *gin.Context) {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"message": "পাতাটি খুঁজে পাওয়া যায়নি"})
	})

	// ---- Public routes ----
	r.GET("/", publicH.Home)
	r.GET("/article/:id", publicH.ArticleDetail)
	r.GET("/health", publicH.HealthCheck)
	r.GET("/api/latest-check", publicH.LatestCheck)

	// ---- Admin auth routes (unprotected) ----
	admin := r.Group("/admin")
	admin.GET("/login", adminH.LoginPage)
	admin.POST("/login", newLoginLimiter().Middleware(), adminH.Login)

	// ---- Admin protected routes ----
	protected := admin.Group("")
	protected.Use(AuthMiddleware(authUC))
	protected.Use(CSRFMiddleware())
	{
		protected.GET("/dashboard", adminH.Dashboard)
		protected.GET("/password", adminH.PasswordPage)
		protected.POST("/password", adminH.ChangePassword)
		protected.POST("/cron/run", adminH.RunCronNow)

		protected.GET("/articles", adminH.ArticlesList)
		protected.POST("/articles/:id/delete", adminH.DeleteArticle)
		protected.POST("/articles/:id/status", adminH.UpdateArticleStatus)

		protected.GET("/feeds", adminH.FeedsList)
		protected.POST("/feeds", adminH.CreateFeed)
		protected.POST("/feeds/:id/delete", adminH.DeleteFeed)
		protected.POST("/feeds/:id/toggle", adminH.ToggleFeed)

		protected.POST("/logout", adminH.Logout)
	}

	return r
}
