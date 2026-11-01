# Cross-compiled desktop builds — no Docker (or Go) needed on the machines
# that run them: the binary self-seeds its data folder on first boot.
# Binaries land in dist/; CGO off = fully static. GOOS/GOARCH/CGO are
# exported per target by make ITSELF (target-specific variables), never by
# shell env-prefix syntax, so `make compileWindows` works from PowerShell
# and CMD as well as from Git Bash. Recipe lines stay portable the same
# way: `mkdir <one-arg>` exists in CMD and POSIX, and clean lists both
# shells' remove with errors ignored (-).

DIST := dist

export CGO_ENABLED := 0

compileWindows:     export GOOS   := windows
compileWindows:     export GOARCH := amd64
compileMacos-arm64: export GOOS   := darwin
compileMacos-arm64: export GOARCH := arm64
compileMacos-amd64: export GOOS   := darwin
compileMacos-amd64: export GOARCH := amd64

.PHONY: compileWindows compileMacos compileMacos-arm64 compileMacos-amd64 all clean

compileWindows: $(DIST)
	go build -trimpath -ldflags="-s -w" -o $(DIST)/dashboard-windows-amd64.exe ./cmd/dashboard

# Apple Silicon + Intel
compileMacos: compileMacos-arm64 compileMacos-amd64

compileMacos-arm64: $(DIST)
	go build -trimpath -ldflags="-s -w" -o $(DIST)/dashboard-macos-arm64 ./cmd/dashboard

compileMacos-amd64: $(DIST)
	go build -trimpath -ldflags="-s -w" -o $(DIST)/dashboard-macos-amd64 ./cmd/dashboard

all: compileWindows compileMacos

$(DIST):
	-mkdir $(DIST)

clean:
	-rm -rf $(DIST)
	-rmdir /s /q $(DIST)
