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
# Data folder: ./data holds everything the dashboard owns — config.yml,
# resources/icons (URL icon cache) and resources/styles (+ overwrite.yml,
# fonts). First boot seeds a sample config + starter styles; existing files
# are never overwritten. Edit through the dashboard's edit buttons or
# directly in ./data.
set -eu

REPO="Paydogs/Homelab-Dashboard"
BRANCH="master"

# compose runs the container as these ids, so ./data must be owned by them
DASH_UID="$(id -u)"
DASH_GID="$(id -g)"
export DASH_UID DASH_GID

# migrate the pre-1.0 layout (./config) into the ./data folder once
if [ -d config ] && [ ! -d data ]; then
  mv config data
  echo "→ migrated existing ./config to ./data"
fi
mkdir -p data

echo "→ downloading docker-compose.yml from github.com/$REPO ($BRANCH)…"
curl -fsSL "https://raw.githubusercontent.com/$REPO/$BRANCH/docker-compose.yml" -o docker-compose.yml

echo "→ starting dashboard (docker builds the image from the repo)…"
docker compose up -d --build

echo
echo "Dashboard starting on http://localhost:8080"
echo "Data (config + styles) lives in ./data — edit via the web UI or directly."