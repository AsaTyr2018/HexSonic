package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr           string
	HTTPReadTimeout    time.Duration
	HTTPWriteTimeout   time.Duration
	HTTPIdleTimeout    time.Duration
	DatabaseURL        string
	RedisAddr          string
	RedisPassword      string
	RedisDB            int
	PrometheusURL      string
	GrafanaURL         string
	PrometheusProxyURL string
	GrafanaProxyURL    string
	PrometheusPublic   string
	GrafanaPublic      string
	StorageRoot        string
	TempRoot           string
	SigningKey         string
	SignedURLTTL       time.Duration
	MaxUploadSizeBytes int64
	FFmpegBin          string
	FFprobeBin         string
	EnableDerivedSync  bool
	AuthRequired       bool
	OIDCIssuerURL      string
	OIDCAudience       string
	OIDCClientID       string
	OIDCClientSecret   string
	OIDCScopes         string
	OIDCAdminUser      string
	OIDCAdminPassword  string
	SubsonicSecretKey  string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:           getEnv("HEXSONIC_HTTP_ADDR", ":8080"),
		HTTPReadTimeout:    getEnvDuration("HEXSONIC_HTTP_READ_TIMEOUT", 15*time.Minute),
		HTTPWriteTimeout:   getEnvDuration("HEXSONIC_HTTP_WRITE_TIMEOUT", 30*time.Minute),
		HTTPIdleTimeout:    getEnvDuration("HEXSONIC_HTTP_IDLE_TIMEOUT", 2*time.Minute),
		DatabaseURL:        getEnv("HEXSONIC_DATABASE_URL", "postgres://hexsonic:hexsonic@localhost:5432/hexsonic?sslmode=disable"),
		RedisAddr:          getEnv("HEXSONIC_REDIS_ADDR", "localhost:6379"),
		RedisPassword:      getEnv("HEXSONIC_REDIS_PASSWORD", ""),
		RedisDB:            getEnvInt("HEXSONIC_REDIS_DB", 0),
		PrometheusURL:      getEnv("HEXSONIC_PROMETHEUS_URL", "http://prometheus:9090/prometheus"),
		GrafanaURL:         getEnv("HEXSONIC_GRAFANA_URL", "http://grafana:3000"),
		PrometheusProxyURL: getEnv("HEXSONIC_PROMETHEUS_PROXY_URL", "http://oauth2-proxy:4180"),
		GrafanaProxyURL:    getEnv("HEXSONIC_GRAFANA_PROXY_URL", "http://grafana:3000"),
		PrometheusPublic:   getEnv("HEXSONIC_PROMETHEUS_PUBLIC_URL", ""),
		GrafanaPublic:      getEnv("HEXSONIC_GRAFANA_PUBLIC_URL", ""),
		StorageRoot:        getEnv("HEXSONIC_STORAGE_ROOT", "./data"),
		TempRoot:           getEnv("HEXSONIC_TEMP_ROOT", "./data/temp"),
		SigningKey:         getEnv("HEXSONIC_SIGNING_KEY", ""),
		SignedURLTTL:       getEnvDuration("HEXSONIC_SIGNED_URL_TTL", 15*time.Minute),
		MaxUploadSizeBytes: getEnvInt64("HEXSONIC_MAX_UPLOAD_BYTES", 2*1024*1024*1024),
		FFmpegBin:          getEnv("HEXSONIC_FFMPEG_BIN", "ffmpeg"),
		FFprobeBin:         getEnv("HEXSONIC_FFPROBE_BIN", "ffprobe"),
		EnableDerivedSync:  getEnvBool("HEXSONIC_ENABLE_DERIVED_SYNC", false),
		AuthRequired:       getEnvBool("HEXSONIC_AUTH_REQUIRED", true),
		OIDCIssuerURL:      getEnv("HEXSONIC_OIDC_ISSUER_URL", "http://localhost:18081/realms/hexsonic"),
		OIDCAudience:       getEnv("HEXSONIC_OIDC_AUDIENCE", ""),
		OIDCClientID:       getEnv("HEXSONIC_OIDC_CLIENT_ID", "hexsonic-api"),
		OIDCClientSecret:   getEnv("HEXSONIC_OIDC_CLIENT_SECRET", ""),
		OIDCScopes:         getEnv("HEXSONIC_OIDC_SCOPES", "openid profile email offline_access"),
		OIDCAdminUser:      getEnv("HEXSONIC_OIDC_ADMIN_USER", ""),
		OIDCAdminPassword:  getEnv("HEXSONIC_OIDC_ADMIN_PASSWORD", ""),
		SubsonicSecretKey:  getEnv("HEXSONIC_SUBSONIC_SECRET_KEY", ""),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("HEXSONIC_DATABASE_URL must be set")
	}
	if cfg.SigningKey == "" {
		return Config{}, fmt.Errorf("HEXSONIC_SIGNING_KEY must be set")
	}
	if cfg.AuthRequired {
		if cfg.OIDCClientSecret == "" {
			return Config{}, fmt.Errorf("HEXSONIC_OIDC_CLIENT_SECRET must be set when auth is enabled")
		}
		if cfg.OIDCAdminUser == "" {
			return Config{}, fmt.Errorf("HEXSONIC_OIDC_ADMIN_USER must be set when auth is enabled")
		}
		if cfg.OIDCAdminPassword == "" {
			return Config{}, fmt.Errorf("HEXSONIC_OIDC_ADMIN_PASSWORD must be set when auth is enabled")
		}
	}
	return cfg, nil
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getEnvInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		i, err := strconv.Atoi(v)
		if err == nil {
			return i
		}
	}
	return def
}

func getEnvInt64(k string, def int64) int64 {
	if v := os.Getenv(k); v != "" {
		i, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			return i
		}
	}
	return def
}

func getEnvDuration(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return def
}

func getEnvBool(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return def
}
