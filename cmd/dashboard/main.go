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

func main() {
	configPath := flag.String("config", "config.yml", "path to the YAML config file")
	listen := flag.String("listen", ":8080", "HTTP listen address")
	flag.Parse()

	logger := log.New(os.Stdout, "[dashboard] ", log.LstdFlags)

	mgr, err := config.NewManager(*configPath, logger)
	if err != nil {
		logger.Fatalf("loading config: %v", err)
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
