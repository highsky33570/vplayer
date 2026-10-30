package config

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tycdn/vplayer/internal/media"
)

func filepathJoin(elem ...string) string { return filepath.Join(elem...) }

// loadDotEnv loads KEY=VALUE pairs without overriding existing env.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" {
			continue
		}
		if _, exists := os.LookupEnv(k); exists {
			continue
		}
		_ = os.Setenv(k, v)
	}
}

type Config struct {
	AppEnv        string
	HTTPAddr      string
	CORSOrigins   []string
	MySQLDSN      string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	R2AccountID   string
	R2AccessKey   string
	R2SecretKey   string
	R2Bucket      string
	R2Endpoint    string
	R2Region      string
	R2PublicBase  string
	// MediaCDNBaseURL is the browser-facing media delivery host (no trailing slash).
	// Falls back to CDNBaseURL when empty.
	MediaCDNBaseURL string
	CDNBaseURL      string
	// MediaCDNPathMode: bucket_prefix (default) | object_key — see media.CDNPathMode.
	MediaCDNPathMode media.CDNPathMode
	// CDNURLAuthMode: none (default) | legacy_hmac
	CDNURLAuthMode string
	CDNSignSecret  string
	// AuthSecret signs user session Bearer tokens (separate from CDN URL/ticket signing).
	AuthSecret string
	// AuthRequired gates consumer account features (login UI, favorites, history,
	// messages, account, comment posting). Default true. false keeps the dormant
	// guest-mode UI. It does not by itself decide playback.
	AuthRequired bool
	// PlaybackAuthRequired gates GET/POST /videos/:id/play. Default true.
	// Production can set false so playback is public while AUTH_REQUIRED stays true.
	// Playback still requires a user when both flags are true. Guest mode
	// (AUTH_REQUIRED=false) keeps playback public so that path stays intact.
	PlaybackAuthRequired bool
	PlayTicketTTL        int
	HLSKeySecret         string
	// MediaSigningEnabled serves R2 playback through short-lived HMAC URLs.
	// Default false so existing CDN URLs stay until production turns it on.
	MediaSigningEnabled bool
	// MediaSigningSecret is HMAC key material. Never sent to clients. Required when signing is enabled.
	MediaSigningSecret string
	// MediaURLTTLSeconds is clamped to 30–3600. Default 600.
	MediaURLTTLSeconds int
	// MediaPublicBase is the browser-facing origin for signed media URLs (no trailing slash).
	// Example: https://example.com. Empty uses the incoming request host.
	MediaPublicBase string

	// Email verification / SMTP (registration only; login does not require these).
	// EmailVerificationEnabled defaults false so registration works without SMTP.
	EmailVerificationEnabled bool
	SMTPHost                 string
	SMTPPort                 int
	SMTPUsername             string
	SMTPPassword             string
	SMTPFromEmail            string
	SMTPFromName             string
	// SMTPTLSServerName overrides TLS SNI/verify name (empty = SMTPHost).
	SMTPTLSServerName string
	// SMTPTLSInsecureSkipVerify allows self-hosted SMTP with a mismatched cert.
	SMTPTLSInsecureSkipVerify bool
	EmailCodePepper            string

	AdminAPIToken string

	OleHdTvSyncEnabled       bool
	OleHdTvSyncIntervalMin   int
	OleHdTvSourceMode        string
	OleHdTvMacCMSDSN         string
	OleHdTvExportPath        string
	OleHdTvFixturePath       string
	OleHdTvHTMLDumpRoot      string
	OleHdTvPublicBaseURL     string
	OleHdTvFetchPosters      bool
	OleHdTvLocalObjectsDir   string
	OleHdTvMediaPublicBase   string
	OleHdTvRequestDelayMs    int
	OleHdTvRequestTimeoutSec int
	OleHdTvMaxConcurrency    int
	OleHdTvMaxRetries        int
	// OleHdTvAuthCookie is an optional opaque Cookie header for authorized OLEHDTV
	// play-page enrichment (OLEHDTV_AUTH_COOKIE). Empty = anonymous (default).
	// Never log, never return via API, never store in MySQL, never commit.
	OleHdTvAuthCookie string

	// EPUSDT / GMPay crypto checkout for 大会员 purchases.
	EpusdtBaseURL           string
	EpusdtCreateOrderPath   string
	EpusdtQueryOrderPath    string
	EpusdtPID               string
	EpusdtAPIToken          string
	EpusdtNotifyURL         string
	EpusdtRedirectURL       string
	EpusdtDefaultCurrency   string
	EpusdtDefaultToken      string
	EpusdtDefaultNetwork    string
	EpusdtOrderTTLMinutes   int
	EpusdtReconcileMinutes  int
}

// loadEnvFiles applies dotenv files without overriding keys already present in
// the process environment. Among files, backend/.env wins over the repo-root
// .env so local backend credentials are not shadowed by root placeholders.
func loadEnvFiles() {
	loadDotEnv(filepathJoin("backend", ".env")) // when cwd is the repo root
	loadDotEnv(".env")                          // backend/.env when cwd is backend/
	loadDotEnv(filepathJoin("..", ".env"))      // repo-root fallback when cwd is backend/
}

func Load() Config {
	loadEnvFiles()
	pathMode, err := media.ParseCDNPathMode(os.Getenv("MEDIA_CDN_PATH_MODE"))
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	return Config{
		AppEnv:               getenv("APP_ENV", "local"),
		HTTPAddr:             getenv("HTTP_ADDR", ":8080"),
		CORSOrigins:          strings.Split(getenv("CORS_ORIGINS", "http://localhost:5173"), ","),
		MySQLDSN:             getenv("MYSQL_DSN", "vplayer:vplayer@tcp(127.0.0.1:3306)/vplayer?charset=utf8mb4&parseTime=True&loc=Local"),
		RedisAddr:            getenv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword:        getenv("REDIS_PASSWORD", ""),
		RedisDB:              getenvInt("REDIS_DB", 0),
		R2AccountID:          getenv("R2_ACCOUNT_ID", ""),
		R2AccessKey:          getenv("R2_ACCESS_KEY_ID", ""),
		R2SecretKey:          getenv("R2_SECRET_ACCESS_KEY", ""),
		R2Bucket:             getenv("R2_BUCKET", "dongman"),
		R2Endpoint:           getenv("R2_ENDPOINT", ""),
		R2Region:             getenv("R2_REGION", "auto"),
		R2PublicBase:         strings.TrimRight(getenv("R2_PUBLIC_BASE", ""), "/"),
		MediaCDNBaseURL:      strings.TrimRight(firstNonEmpty(getenv("MEDIA_CDN_BASE_URL", ""), getenv("CDN_BASE_URL", "http://localhost:8080")), "/"),
		CDNBaseURL:           strings.TrimRight(getenv("CDN_BASE_URL", "http://localhost:8080"), "/"),
		MediaCDNPathMode:     pathMode,
		CDNURLAuthMode:       strings.ToLower(getenv("CDN_URL_AUTH_MODE", "none")),
		CDNSignSecret:        getenv("CDN_SIGN_SECRET", "dev-cdn-secret"),
		AuthSecret:           getenv("AUTH_SECRET", "dev-auth-secret"),
		AuthRequired:         getenvBool("AUTH_REQUIRED", true),
		PlaybackAuthRequired: getenvBool("PLAYBACK_AUTH_REQUIRED", true),
		PlayTicketTTL:        getenvInt("PLAY_TICKET_TTL_SEC", 300),
		HLSKeySecret:         getenv("HLS_KEY_SECRET", "dev-hls-key-secret"),
		MediaSigningEnabled:  getenvBool("MEDIA_SIGNING_ENABLED", false),
		MediaSigningSecret:   os.Getenv("MEDIA_SIGNING_SECRET"),
		MediaURLTTLSeconds:   clampMediaTTL(getenvInt("MEDIA_URL_TTL_SECONDS", 600)),
		MediaPublicBase:      strings.TrimRight(getenv("MEDIA_PUBLIC_BASE", ""), "/"),

		EmailVerificationEnabled:   getenvBool("EMAIL_VERIFICATION_ENABLED", false),
		SMTPHost:                   getenv("SMTP_HOST", ""),
		SMTPPort:                   getenvInt("SMTP_PORT", 587),
		SMTPUsername:               getenv("SMTP_USERNAME", ""),
		SMTPPassword:               getenv("SMTP_PASSWORD", ""),
		SMTPFromEmail:              getenv("SMTP_FROM_EMAIL", ""),
		SMTPFromName:               getenv("SMTP_FROM_NAME", "天涯影视"),
		SMTPTLSServerName:         getenv("SMTP_TLS_SERVER_NAME", ""),
		SMTPTLSInsecureSkipVerify: getenvBool("SMTP_TLS_INSECURE_SKIP_VERIFY", false),
		EmailCodePepper:            getenv("EMAIL_CODE_PEPPER", ""),

		AdminAPIToken: getenv("ADMIN_API_TOKEN", "dev-admin-token"),

		OleHdTvSyncEnabled:       getenvBool("OLEHDTV_SYNC_ENABLED", false),
		OleHdTvSyncIntervalMin:   getenvInt("OLEHDTV_SYNC_INTERVAL_MINUTES", 15),
		OleHdTvSourceMode:        getenv("OLEHDTV_SOURCE_MODE", "fixture"),
		OleHdTvMacCMSDSN:         getenv("OLEHDTV_MACCMS_DSN", ""),
		OleHdTvExportPath:        getenv("OLEHDTV_EXPORT_PATH", ""),
		OleHdTvFixturePath:       getenv("OLEHDTV_FIXTURE_PATH", "testdata/olehdtv_sample.json"),
		OleHdTvHTMLDumpRoot:      getenv("OLEHDTV_HTML_DUMP_ROOT", "testdata/maccms_html"),
		OleHdTvPublicBaseURL:     strings.TrimRight(getenv("OLEHDTV_PUBLIC_BASE_URL", "https://maccms.local"), "/"),
		OleHdTvFetchPosters:      getenvBool("OLEHDTV_FETCH_POSTERS", false),
		OleHdTvLocalObjectsDir:   getenv("OLEHDTV_LOCAL_OBJECTS_DIR", "data/objects"),
		OleHdTvMediaPublicBase:   strings.TrimRight(getenv("OLEHDTV_MEDIA_PUBLIC_BASE", "http://localhost:8080/media"), "/"),
		OleHdTvRequestDelayMs:    getenvInt("OLEHDTV_REQUEST_DELAY_MS", 1000),
		OleHdTvRequestTimeoutSec: getenvInt("OLEHDTV_REQUEST_TIMEOUT_SECONDS", 20),
		OleHdTvMaxConcurrency:    getenvInt("OLEHDTV_MAX_CONCURRENCY", 2),
		OleHdTvMaxRetries:        getenvInt("OLEHDTV_MAX_RETRIES", 3),
		OleHdTvAuthCookie:        strings.TrimSpace(os.Getenv("OLEHDTV_AUTH_COOKIE")),

		EpusdtBaseURL:          strings.TrimRight(getenv("EPUSDT_BASE_URL", ""), "/"),
		EpusdtCreateOrderPath:  getenv("EPUSDT_CREATE_ORDER_PATH", "/payments/gmpay/v1/order/create-transaction"),
		EpusdtQueryOrderPath:   getenv("EPUSDT_QUERY_ORDER_PATH", ""),
		EpusdtPID:              strings.TrimSpace(getenv("EPUSDT_PID", "")),
		EpusdtAPIToken:         getenv("EPUSDT_API_TOKEN", ""),
		EpusdtNotifyURL:        strings.TrimSpace(getenv("EPUSDT_NOTIFY_URL", "")),
		EpusdtRedirectURL:      strings.TrimSpace(getenv("EPUSDT_REDIRECT_URL", "")),
		EpusdtDefaultCurrency:  strings.ToLower(getenv("EPUSDT_DEFAULT_CURRENCY", "usd")),
		EpusdtDefaultToken:     strings.ToLower(getenv("EPUSDT_DEFAULT_TOKEN", "usdt")),
		EpusdtDefaultNetwork:   strings.ToLower(getenv("EPUSDT_DEFAULT_NETWORK", "tron")),
		EpusdtOrderTTLMinutes:  getenvInt("EPUSDT_ORDER_TTL_MINUTES", 30),
		EpusdtReconcileMinutes: getenvInt("EPUSDT_RECONCILE_MINUTES", 5),
	}
}

// OleHdTvAuthConfigured reports whether optional OLEHDTV auth is present without exposing it.
func (c Config) OleHdTvAuthConfigured() bool {
	return strings.TrimSpace(c.OleHdTvAuthCookie) != ""
}

const (
	mediaURLTTLMin = 30
	mediaURLTTLMax = 3600
)

func clampMediaTTL(n int) int {
	if n < mediaURLTTLMin {
		return mediaURLTTLMin
	}
	if n > mediaURLTTLMax {
		return mediaURLTTLMax
	}
	return n
}

// CheckMediaSigning fails closed when signed delivery is on without a usable secret.
func (c Config) CheckMediaSigning() error {
	if !c.MediaSigningEnabled {
		return nil
	}
	if len(strings.TrimSpace(c.MediaSigningSecret)) < 16 {
		return fmt.Errorf("media signing enabled but MEDIA_SIGNING_SECRET is missing or too short")
	}
	if c.MediaURLTTLSeconds < mediaURLTTLMin || c.MediaURLTTLSeconds > mediaURLTTLMax {
		return fmt.Errorf("media signing enabled but MEDIA_URL_TTL_SECONDS is outside %d-%d", mediaURLTTLMin, mediaURLTTLMax)
	}
	return nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getenvBool(k string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(k)))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// RegistrationMailConfigured reports whether SMTP + code pepper are set for
// verified registration. Login/API startup must not depend on this; missing
// mail config only disables register/start|resend with a controlled error.
func (c Config) RegistrationMailConfigured() bool {
	return strings.TrimSpace(c.SMTPHost) != "" &&
		c.SMTPPort > 0 &&
		strings.TrimSpace(c.SMTPFromEmail) != "" &&
		strings.TrimSpace(c.EmailCodePepper) != "" &&
		strings.TrimSpace(c.SMTPUsername) != "" &&
		strings.TrimSpace(c.SMTPPassword) != ""
}
