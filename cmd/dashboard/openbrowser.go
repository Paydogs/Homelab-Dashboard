package main

import (
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Desktop convenience: once the server is up, open the dashboard in a
// window of its own — a chromeless --app window when a Chromium browser
// (Chrome/Edge/Brave) is installed, else a normal default-browser tab.
// Best effort by design: a failure is logged, never fatal, and the server
// keeps running regardless. Linux keeps this off by default (the Docker
// deployment must never try to spawn a browser).

// openAfterListen waits until the address actually accepts connections,
// then opens the dashboard URL — the browser must never get there first.
func openAfterListen(listen string, logger *log.Logger) {
	waitForListen(dialAddr(listen))
	openURL(dashboardURL(listen), logger)
}

// waitForListen polls until addr accepts TCP (bounded to ~5s: after that,
// open anyway — the user would rather see the URL than nothing).
func waitForListen(addr string) {
	for range 100 {
		c, err := net.DialTimeout("tcp", addr, 150*time.Millisecond)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// dialAddr is the address to probe; a wildcard listen is probed via loopback.
func dialAddr(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil { // bare port ("8080") or bare host — treat as ":"+listen
		if _, err := net.LookupPort("tcp", listen); err == nil {
			return "127.0.0.1:" + listen
		}
		return "127.0.0.1:8080"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// dashboardURL is the browser-facing URL for the listen address.
func dashboardURL(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "http://localhost:" + listen
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// openURL prefers a dedicated app window, falling back to the default browser.
func openURL(url string, logger *log.Logger) {
	if cmd := appWindowCmd(url); cmd != nil {
		if err := cmd.Start(); err == nil {
			logger.Printf("opened %s in an app window", url)
			return
		}
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		logger.Printf("opening browser for %s: %v", url, err)
	}
}

// appWindowCmd builds a Chromium --app=<url> launch (a real window without
// browser chrome — the desktop feel), or nil when no Chromium browser is
// installed (Firefox has no app mode; Safari neither).
func appWindowCmd(url string) *exec.Cmd {
	for _, browser := range chromiumCandidates() {
		if _, err := os.Stat(browser); err == nil {
			return exec.Command(browser, "--app="+url)
		}
	}
	return nil
}

// chromiumCandidates lists well-known install locations; browsers are
// deliberately NOT searched in PATH (they rarely live there).
func chromiumCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		var paths []string
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
			d := os.Getenv(env)
			if d == "" {
				continue
			}
			paths = append(paths,
				filepath.Join(d, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(d, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(d, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
			)
		}
		return paths
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	}
	return nil
}
