// Command goboxd is a sandboxed code execution service.
// It receives source code via HTTP, runs it inside an nsjail namespace,
// and returns the result. See docs/architecture.md for the full design.
package main

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/thesouldev/goboxd/internal/config"
	"github.com/thesouldev/goboxd/internal/handler"
	"github.com/thesouldev/goboxd/internal/runner"
	"github.com/thesouldev/goboxd/internal/sandbox"
)

// Version and GitCommit are injected at build time via:
//
//	-ldflags="-X main.Version=0.1.0 -X main.GitCommit=$(git rev-parse --short HEAD)"
var (
	Version   = "0.1.0"
	GitCommit = "unknown"
)

func main() {
	// --- Logger setup ---
	logLevel := zapcore.InfoLevel
	if os.Getenv("LOG_LEVEL") == "debug" {
		logLevel = zapcore.DebugLevel
	}
	zapCfg := zap.NewProductionConfig()
	zapCfg.Level = zap.NewAtomicLevelAt(logLevel)
	logger, err := zapCfg.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck

	// --- Configuration ---
	cfgPath := envOrDefault("CONFIG_PATH", "/app/configs/languages.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		logger.Fatal("failed to load language config", zap.Error(err))
	}
	logger.Info("language config loaded", zap.Int("language_count", len(cfg.ByID)))

	// --- Jail directory ---
	jailDir := envOrDefault("JAIL_DIR", "/var/jails")

	// HOLE 7 FIX: Sweep stale jail directories from previous runs / crashes.
	sandbox.SweepStale(jailDir, logger)

	// --- Concurrency ---
	maxJobs := runtime.NumCPU()
	if v := os.Getenv("MAX_CONCURRENT_JOBS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxJobs = n
		}
	}
	logger.Info("concurrency configured", zap.Int("max_concurrent_jobs", maxJobs))

	// --- Runner ---
	r := runner.New(cfg, jailDir, maxJobs, logger)

	// Log all language versions at startup.
	for _, lang := range cfg.All() {
		ver, err := runner.LanguageVersion(lang.VersionCmd)
		if err != nil {
			logger.Warn("language toolchain not found",
				zap.String("language", lang.ID),
				zap.Error(err),
			)
		} else {
			logger.Info("language toolchain ready",
				zap.String("language", lang.ID),
				zap.String("version", ver),
			)
		}
	}

	// --- Handlers ---
	runHandler := &handler.RunHandler{
		Runner: r,
		Config: cfg,
		Logger: logger,
	}
	infoHandler := &handler.InfoHandler{
		Runner:    r,
		Config:    cfg,
		JailDir:   jailDir,
		MaxJobs:   maxJobs,
		Version:   Version,
		GitCommit: GitCommit,
		Logger:    logger,
	}
	readyzHandler := &handler.ReadyzHandler{
		Config: cfg,
		Logger: logger,
	}

	// --- Router ---
	mux := chi.NewRouter()
	mux.Use(middleware.Recoverer)
	mux.Use(middleware.RealIP)

	mux.Post("/run", runHandler.ServeHTTP)
	mux.Get("/healthz", handler.HealthHandler)
	mux.Get("/readyz", readyzHandler.ServeHTTP)
	mux.Get("/info", infoHandler.ServeHTTP)

	// --- Server ---
	addr := envOrDefault("LISTEN_ADDR", ":8080")
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	logger.Info("goboxd starting",
		zap.String("addr", addr),
		zap.String("version", Version),
		zap.String("commit", GitCommit),
		zap.String("jail_dir", jailDir),
		zap.Int("max_concurrent_jobs", maxJobs),
	)

	if err := srv.ListenAndServe(); err != nil {
		logger.Fatal("server exited", zap.Error(err))
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
