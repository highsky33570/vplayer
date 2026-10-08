package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	// MediaCDNBaseURL is the browser-facing TYCDN/CDNfly host (no trailing slash).
	// Falls back to CDNBaseURL when empty.
	MediaCDNBaseURL string
	CDNBaseURL      string
	// CDNURLAuthMode: none (default; CDNfly origin auth to private R2) | legacy_hmac
	CDNURLAuthMode string
	CDNSignSecret  string
	PlayTicketTTL  int
	HLSKeySecret   string

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
	return Config{
		AppEnv:        getenv("APP_ENV", "local"),
		HTTPAddr:      getenv("HTTP_ADDR", ":8080"),
		CORSOrigins:   strings.Split(getenv("CORS_ORIGINS", "http://localhost:5173"), ","),
		MySQLDSN:      getenv("MYSQL_DSN", "vplayer:vplayer@tcp(127.0.0.1:3306)/vplayer?charset=utf8mb4&parseTime=True&loc=Local"),
		RedisAddr:     getenv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword: getenv("REDIS_PASSWORD", ""),
		RedisDB:       getenvInt("REDIS_DB", 0),
		R2AccountID:     getenv("R2_ACCOUNT_ID", ""),
		R2AccessKey:     getenv("R2_ACCESS_KEY_ID", ""),
		R2SecretKey:     getenv("R2_SECRET_ACCESS_KEY", ""),
		R2Bucket:        getenv("R2_BUCKET", "dongman"),
		R2Endpoint:      getenv("R2_ENDPOINT", ""),
		R2Region:        getenv("R2_REGION", "auto"),
		R2PublicBase:    strings.TrimRight(getenv("R2_PUBLIC_BASE", ""), "/"),
		MediaCDNBaseURL: strings.TrimRight(firstNonEmpty(getenv("MEDIA_CDN_BASE_URL", ""), getenv("CDN_BASE_URL", "http://localhost:8080")), "/"),
		CDNBaseURL:      strings.TrimRight(getenv("CDN_BASE_URL", "http://localhost:8080"), "/"),
		CDNURLAuthMode:  strings.ToLower(getenv("CDN_URL_AUTH_MODE", "none")),
		CDNSignSecret:   getenv("CDN_SIGN_SECRET", "dev-cdn-secret"),
		PlayTicketTTL:   getenvInt("PLAY_TICKET_TTL_SEC", 300),
		HLSKeySecret:    getenv("HLS_KEY_SECRET", "dev-hls-key-secret"),

		AdminAPIToken: getenv("ADMIN_API_TOKEN", "dev-admin-token"),

		OleHdTvSyncEnabled:     getenvBool("OLEHDTV_SYNC_ENABLED", false),
		OleHdTvSyncIntervalMin: getenvInt("OLEHDTV_SYNC_INTERVAL_MINUTES", 15),
		OleHdTvSourceMode:      getenv("OLEHDTV_SOURCE_MODE", "fixture"),
		OleHdTvMacCMSDSN:       getenv("OLEHDTV_MACCMS_DSN", ""),
		OleHdTvExportPath:      getenv("OLEHDTV_EXPORT_PATH", ""),
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
	}
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
