#!/bin/sh
# Homelab dashboard installer/updater.
#
# One-liner (run in an empty directory on the VM):
#   sh -c "$(curl -fsSL https://raw.githubusercontent.com/Paydogs/Homelab-Dashboard/master/install.sh)"
#
# It downloads docker-compose.yml into the current directory and brings the
# stack up (building the image from the repo inside Docker). Re-running it
# later updates: fresh code from GitHub, `docker compose up -d --build`.
#
# First boot seeds ./config with a sample config.yml + starter styles
# (existing files are never overwritten) — edit through the dashboard's
# edit buttons or in ./config afterwards.
set -eu

REPO="Paydogs/Homelab-Dashboard"
BRANCH="master"

# compose runs the container as these ids, so ./config must be owned by them
DASH_UID="$(id -u)"
DASH_GID="$(id -g)"
export DASH_UID DASH_GID
mkdir -p config

echo "→ downloading docker-compose.yml from github.com/$REPO ($BRANCH)…"
curl -fsSL "https://raw.githubusercontent.com/$REPO/$BRANCH/docker-compose.yml" -o docker-compose.yml

echo "→ starting dashboard (docker builds the image from the repo)…"
docker compose up -d --build

echo
echo "Dashboard starting on http://localhost:8080"
echo "Config + styles were seeded into ./config — edit them via the web UI or directly."