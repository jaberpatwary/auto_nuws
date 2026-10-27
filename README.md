# বাংলা নিউজ অ্যাগ্রিগেটর — Go + Gin + PostgreSQL + AI

Go দিয়ে তৈরি RSS-ভিত্তিক বাংলা নিউজ অ্যাগ্রিগেটর MVP। এটি RSS/Atom ফিড থেকে খবর সংগ্রহ করে, Groq দিয়ে সারসংক্ষেপ তৈরির চেষ্টা করে এবং ব্যর্থ হলে Gemini ব্যবহার করে। এরপর নিবন্ধ PostgreSQL-এ সংরক্ষণ করে। পাঠকদের জন্য Public Website এবং ফিড/নিবন্ধ পরিচালনার জন্য JWT-সুরক্ষিত Admin Panel রয়েছে।

> **গুরুত্বপূর্ণ:** এই রিপোজিটরি deploy করার মতো করে সাজানো হয়েছে, কিন্তু এই পরিবেশে ইন্টারনেট/DNS বন্ধ থাকায় Go dependencies ডাউনলোড করে compile/test চালানো যায়নি। আপনার কম্পিউটার বা CI-তে `go mod tidy`, `go test ./...` এবং deployment smoke test চালানো বাধ্যতামূলক। RSS provider URL ও AI model availability-ও deploy-এর আগে যাচাই করুন।

## ফিচার

- Public homepage, article detail page এবং category filter
- Admin login, dashboard, article publish/archive/delete
- RSS feed add, activate/deactivate, delete
- Parallel RSS feed fetching এবং source URL দিয়ে duplicate detection
- Groq primary + Gemini fallback summarisation
- AI usage logging
- PostgreSQL + GORM AutoMigrate
- Dockerfile, Docker Compose, Render Blueprint এবং GitHub Actions CI
- Health endpoint: `/health`
- Admin login-এ per-process brute-force rate limit
- Production-এ শক্তিশালী JWT secret ও admin password বাধ্যতামূলক
- Secure cookie flags (production), SameSite Strict, basic security headers
- Public route থেকে draft/archived article দেখা যাবে না
- Feed URL validation-এ localhost/private IP-এর সরাসরি URL block করা হয়

## লোকাল মেশিনে চালানো

### দরকার হবে

- Go 1.22 বা নতুন
- Docker Desktop / Docker Engine এবং Docker Compose
- AI সারাংশের জন্য Groq এবং/অথবা Gemini API key (ঐচ্ছিক; key না থাকলে মূল কনটেন্ট থেকে ছোট fallback summary তৈরি হবে)

### Option A: Docker Compose (সহজ উপায়)

1. ZIP extract করে project folder-এ যান।
2. `.env.example` কপি করে `.env` তৈরি করুন:

   ```bash
   cp .env.example .env
   ```

3. `.env`-এ চাইলে `GROQ_API_KEY` এবং `GEMINI_API_KEY` দিন। লোকাল development-এর default admin হলো `admin` / `admin123`; production-এ কখনো এই credential ব্যবহার করবেন না।
4. চালান:

   ```bash
   docker compose up --build
   ```

5. খুলুন:
   - Public website: `http://localhost:8080`
   - Admin login: `http://localhost:8080/admin/login`
   - Health check: `http://localhost:8080/health`

বন্ধ করতে `Ctrl+C`; background-এ চালালে `docker compose down`। ডেটা `pgdata` volume-এ থাকবে।

### Option B: সরাসরি Go দিয়ে

একটি PostgreSQL database তৈরি করুন, `.env.example` কপি করে `.env` বানান এবং `DATABASE_URL` ঠিক করুন। এরপর:

```bash
go mod tidy
go test ./...
go vet ./...
go run ./cmd/server
```

প্রথম `go mod tidy` dependencies download করবে এবং `go.sum` তৈরি করবে। এই ZIP-এ `go.sum` রাখা যায়নি, কারণ build environment থেকে `proxy.golang.org`-এ নেটওয়ার্ক সংযোগ পাওয়া যায়নি।

## Environment variables

| Variable | প্রয়োজন | বর্ণনা |
|---|---|---|
| `PORT` | না | Web port, default `8080` |
| `DATABASE_URL` | হ্যাঁ | PostgreSQL connection string |
| `JWT_SECRET` | হ্যাঁ | JWT signing secret; production-এ কমপক্ষে 32 অক্ষরের random secret |
| `JWT_EXPIRY` | না | JWT expiry, default `24h` |
| `ADMIN_DEFAULT_USER` | না | প্রথম admin username, default `admin` |
| `ADMIN_DEFAULT_PASS` | হ্যাঁ (production) | প্রথমবার seed হওয়া admin password; production-এ কমপক্ষে 12 অক্ষর |
| `GROQ_API_KEY` | না | Groq API key |
| `GROQ_MODEL` | না | Groq model name |
| `GEMINI_API_KEY` | না | Gemini API key |
| `GEMINI_MODEL` | না | Gemini model name |
| `CRON_INTERVAL` | না | RSS polling interval, default `1h` |
| `FEED_WORKER_COUNT` | না | Parallel feed workers, default `5` |
| `APP_ENV` | না | Production-এ `production` দিন |

Production-এ `JWT_SECRET` ও `ADMIN_DEFAULT_PASS` environment variable হিসেবে Render dashboard-এ সেট করুন। Secret/password source control-এ commit করবেন না।

## Render-এ deploy

1. Project GitHub repository-তে push করুন।
2. Render-এ **New → Blueprint** বেছে নিয়ে `render.yaml` ব্যবহার করুন, অথবা Docker Web Service তৈরি করুন।
3. একটি external PostgreSQL database (যেমন Neon বা Supabase) তৈরি করে `DATABASE_URL` দিন। Provider যে SSL parameter দেয় তা অনুসরণ করুন; hosted database-এর ক্ষেত্রে সাধারণত `sslmode=require` লাগে।
4. Render environment-এ `ADMIN_DEFAULT_PASS`-এর জন্য অন্তত 12 অক্ষরের unique password দিন। `GROQ_API_KEY` ও `GEMINI_API_KEY` প্রয়োজন অনুযায়ী দিন।
5. Deploy শেষে `/health`, public homepage, admin login এবং manual “fetch now” action পরীক্ষা করুন।
6. DNS ব্যবহার করলে আপনার domain Cloudflare-এ যুক্ত করে Render-এর নির্দেশিত DNS record সেট করুন।

### Render Free Tier-এর সীমাবদ্ধতা

Render Free Web Service নিষ্ক্রিয় হলে sleep করতে পারে। এই অ্যাপের hourly `time.Ticker` worker একই process-এ চলে; তাই Free Web Service ঘুমালে hourly fetch নির্ভরযোগ্যভাবে চলবে না। নিয়মিত সময়সূচিতে RSS fetch নিশ্চিত করতে persistent/background worker বা external scheduler/cron service ব্যবহার করুন। শুধু uptime ping-কে scheduler হিসেবে নির্ভরযোগ্য সমাধান ধরে নেবেন না।

## RSS feed configuration

প্রথম startup-এ চারটি seed feed যোগ হয়, কিন্তু publisher-দের RSS URL পরিবর্তন হতে পারে। Admin Panel → RSS Feed থেকে প্রতিটি URL পরীক্ষা করে সঠিক URL দিন। RSS item-এর content অসম্পূর্ণ হতে পারে; feed-এ full text না থাকলে summary-ও সীমিত হবে।

## Security checklist before public launch

- [ ] `APP_ENV=production`
- [ ] `JWT_SECRET` unique এবং অন্তত 32 অক্ষর
- [ ] `ADMIN_DEFAULT_PASS` unique এবং অন্তত 12 অক্ষর
- [ ] Database password ও API keys শুধুমাত্র environment variables-এ
- [ ] প্রথম admin login পরীক্ষা করুন; পরে Admin Panel → পাসওয়ার্ড পরিবর্তন থেকে password update করুন
- [ ] HTTPS এবং cookie secure flag পরীক্ষা করুন
- [ ] Login rate limit-এর পাশাপাশি Cloudflare/edge rate limiting যোগ করুন; in-memory limiter একাধিক instance-এ shared নয়
- [ ] RSS URL ও AI model names পরীক্ষা করুন
- [ ] Database backup/retention configure করুন
- [ ] AI-generated summary যাচাই, source attribution এবং publisher-এর copyright/terms নিশ্চিত করুন
- [ ] `go test ./...`, `go vet ./...`, Docker build ও smoke tests চালান

## Architecture

```text
Reader/Admin → Cloudflare (optional) → Go/Gin app
                                         ├─ Public handlers + HTML templates
                                         ├─ Admin handlers + JWT cookie middleware
                                         ├─ Usecases
                                         ├─ GORM repositories → PostgreSQL
                                         └─ RSS worker → Groq → Gemini fallback
```

## Known limitations

- Password reset-by-email feature নেই; account access হারালে database/operations-এর মাধ্যমে নিরাপদ reset করতে হবে।
- Feed URL validation localhost/private IP literal block করে, কিন্তু DNS rebinding/resolve-to-private-address প্রতিরোধের সম্পূর্ণ egress firewall নয়।
- Hourly worker web process-এর সঙ্গে যুক্ত; dependable scheduling-এর জন্য আলাদা worker/scheduler service প্রয়োজন।
- Feed URLs ও AI model names third-party service-এ পরিবর্তিত হতে পারে।
- এই execution environment-এ network নিষেধাজ্ঞার কারণে dependency download/build/test সম্পন্ন করা যায়নি। CI workflow push/pull request-এ dependency resolution, tests এবং vet চালাবে।
