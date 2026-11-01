package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"dashboard/internal/appstate"
)

// flagPassed reports whether a flag was set explicitly on the command line
// (so defaults and remembered choices don't shadow it).
func flagPassed(name string) bool {
	seen := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			seen = true
		}
	})
	return seen
}

func main() {
	dataDir := flag.String("data", ".", "the dashboard's data folder (config.yml + resources/)")
	listen := flag.String("listen", ":8080", "HTTP listen address")
	defaultsDir := flag.String("defaults", "", "folder with first-boot default files (sample config, starter styles); copied when the data folder has no config yet")
	// default on for the desktop OSes (double-clicked exe should show a
	// window), off on Linux where the Docker deployment must not spawn a browser
	openBrowser := flag.Bool("open", runtime.GOOS == "windows" || runtime.GOOS == "darwin",
		"open the dashboard in a browser window once the server is up")
	flag.Parse()

	// data folder resolution: an explicit -data always wins; otherwise a
	// folder once chosen in the web UI (desktop) is remembered in the user
	// config dir; else the working directory.
	if !flagPassed("data") {
		if saved, ok := appstate.Load(); ok {
			*dataDir = saved
		}
	}

	logger := log.New(os.Stdout, "[dashboard] ", log.LstdFlags)

	// Docker passes -defaults /defaults (the image's copy); a plain binary
	// run seeds from the defaults compiled into the executable instead, so
	// Windows/macOS need nothing but the binary itself.
	seed := func() error {
		if *defaultsDir != "" {
			return seedDefaults(*dataDir, *defaultsDir, logger)
		}
		return seedEmbeddedDefaults(*dataDir, logger)
	}

	// The port opens no matter what: with a broken/missing config the boot
	// page names the problem and refreshes until a background retry succeeds.
	boot := &boot{}
	h, err := buildServer(*dataDir, seed, logger)
	if err != nil {
		logger.Printf("waiting for config: %v (serving the boot page, retrying every 5s)", err)
		boot.setStatus(err.Error())
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				h, err := buildServer(*dataDir, seed, logger)
				if err != nil {
					boot.setStatus(err.Error())
					continue
				}
				logger.Printf("config became available, dashboard is live")
				boot.goLive(h)
				return
			}
		}()
	} else {
		boot.goLive(h)
	}

	httpSrv := &http.Server{
		Addr:              *listen,
		Handler:           http.HandlerFunc(boot.root),
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

	if *openBrowser {
		go openAfterListen(*listen, logger)
	}

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Printf("shutdown: %v", err)
	}
}
