package config

import (
	"bufio"
	"log"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL    string
	SMTPHost       string
	SMTPPort       int
	SMTPUser       string
	SMTPPass       string
	SMTPFrom       string
	WebAuthnRPID   string
	WebAuthnOrigin string
	BootstrapEmail  string
	LlamaServerPath string
	LlamaModelPath  string
	ServerAddr      string
}

// loadDotEnv reads a .env file and sets any variables not already present in
// the environment. Lines starting with # are comments. Blank lines are skipped.
// Values may optionally be quoted with single or double quotes.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // .env is optional
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// Strip matching surrounding quotes.
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if key != "" && os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

func Load() *Config {
	loadDotEnv(".env")
	cfg := &Config{
		BootstrapEmail:  env("BOOTSTRAP_EMAIL", ""),
		DatabaseURL:    mustEnv("DATABASE_URL"),
		SMTPHost:       env("SMTP_HOST", "localhost"),
		SMTPPort:       envInt("SMTP_PORT", 587),
		SMTPUser:       env("SMTP_USER", ""),
		SMTPPass:       env("SMTP_PASS", ""),
		SMTPFrom:       env("SMTP_FROM", "noreply@localhost"),
		WebAuthnRPID:   mustEnv("WEBAUTHN_RPID"),
		WebAuthnOrigin: mustEnv("WEBAUTHN_ORIGIN"),
		LlamaServerPath: env("LLAMA_SERVER_PATH", "llama/build/bin/llama-server"),
		LlamaModelPath:  env("LLAMA_MODEL_PATH", ""),
		ServerAddr:     env("SERVER_ADDR", ":8080"),
	}
	return cfg
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %s is not set", key)
	}
	return v
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			log.Fatalf("invalid value for %s: %v", key, err)
		}
		return n
	}
	return def
}
