package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"dashboard/internal/config"
	"dashboard/internal/health"
	"dashboard/internal/server"
	"dashboard/internal/styles"
)

// waitForConfig retries the seeding + initial load every 5s — e.g. while
// the operator fixes the container user's write permissions on the config
// folder — instead of exiting into a restart loop.
func waitForConfig(configPath, defaultsDir string, logger *log.Logger) *config.Manager {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if defaultsDir != "" {
			if err := seedDefaults(configPath, defaultsDir, logger); err != nil {
				logger.Printf("seeding defaults: %v", err)
				continue
			}
		}
		mgr, err := config.NewManager(configPath, logger)
		if err != nil {
			continue
		}
		logger.Printf("config became available, starting")
		return mgr
	}
	return nil // unreachable
}

func main() {
	configPath := flag.String("config", "config.yml", "path to the YAML config file")
	listen := flag.String("listen", ":8080", "HTTP listen address")
	defaultsDir := flag.String("defaults", "", "folder with first-boot default files (sample config, starter styles); copied when no config exists yet")
	flag.Parse()

	logger := log.New(os.Stdout, "[dashboard] ", log.LstdFlags)

	if *defaultsDir != "" {
		if err := seedDefaults(*configPath, *defaultsDir, logger); err != nil {
			logger.Printf("seeding defaults: %v "+
				"(hint: the config folder must be writable by the container user — "+
				"chown it or run install.sh, which matches compose user to your ids)", err)
		}
	}

	mgr, err := config.NewManager(*configPath, logger)
	if err != nil {
		// first boot may simply be waiting for the operator to make the
		// config folder writable — poll instead of crashlooping
		logger.Printf("waiting for config: %v (retrying every 5s until the config folder is ready)", err)
		mgr = waitForConfig(*configPath, *defaultsDir, logger)
	}

	styleMgr := styles.NewManager(filepath.Join(mgr.ResourcesDir(), "styles"), logger)

	engine := health.New()
	srv, err := server.New(mgr, engine, styleMgr, logger)
	if err != nil {
		logger.Fatalf("starting server: %v", err)
	}

	httpSrv := &http.Server{
		Addr:              *listen,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Printf("listening on %s", *listen)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Printf("server error: %v", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Printf("shutdown: %v", err)
	}
}
