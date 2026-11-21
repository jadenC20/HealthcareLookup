package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type runtimeConfig struct {
	Address        string
	StoreMode      string
	StoreDSN       string
	AllowedOrigins map[string]bool
}

func loadRuntimeConfig() (runtimeConfig, error) {
	host, port := "", ""
	address := strings.TrimSpace(os.Getenv("CLEARCARE_ADDR"))
	if address != "" {
		var err error
		host, port, err = net.SplitHostPort(address)
		if err != nil {
			return runtimeConfig{}, fmt.Errorf("CLEARCARE_ADDR must be a valid host:port")
		}
	} else {
		host = strings.TrimSpace(os.Getenv("CLEARCARE_HOST"))
		if host == "" {
			host = "127.0.0.1"
		}
		port = strings.TrimSpace(os.Getenv("PORT"))
		if port == "" {
			port = strings.TrimSpace(os.Getenv("CLEARCARE_PORT"))
		}
		if port == "" {
			port = "8000"
		}
		address = net.JoinHostPort(host, port)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return runtimeConfig{}, fmt.Errorf("PORT/CLEARCARE_PORT must be a valid TCP port")
	}
	mode := strings.TrimSpace(os.Getenv("CLEARCARE_STORE"))
	if mode == "" {
		mode = "local-sqlite"
	}
	dsn := strings.TrimSpace(os.Getenv("CLEARCARE_DB"))
	switch mode {
	case "local-sqlite":
		if dsn == "" {
			dsn = "../clearcare.db"
		}
	case "memory":
		dsn = "file:clearcare-memory?mode=memory&cache=shared"
	case "postgres":
		dsn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
		if dsn == "" {
			return runtimeConfig{}, fmt.Errorf("DATABASE_URL is required for postgres storage")
		}
	default:
		return runtimeConfig{}, fmt.Errorf("CLEARCARE_STORE must be local-sqlite, memory or postgres")
	}
	origins := map[string]bool{}
	rawOrigins := strings.TrimSpace(os.Getenv("CLEARCARE_ALLOWED_ORIGINS"))
	if rawOrigins == "" {
		rawOrigins = "http://127.0.0.1:4173,http://localhost:4173,http://127.0.0.1:4180,http://localhost:4180,http://127.0.0.1:4182,http://localhost:4182,http://127.0.0.1:8000,http://localhost:8000"
	}
	for _, raw := range strings.Split(rawOrigins, ",") {
		origin := strings.TrimSuffix(strings.TrimSpace(raw), "/")
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Path != "" {
			return runtimeConfig{}, fmt.Errorf("invalid origin %q in CLEARCARE_ALLOWED_ORIGINS", raw)
		}
		origins[origin] = true
	}
	return runtimeConfig{Address: address, StoreMode: mode, StoreDSN: dsn, AllowedOrigins: origins}, nil
}

func (c runtimeConfig) originAllowed(origin string) bool {
	return origin == "" || c.AllowedOrigins[strings.TrimSuffix(origin, "/")]
}
