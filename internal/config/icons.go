package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dashboard/domain"
)

// Icon fallback served when an item has no usable icon.
const FallbackIcon = "/static/icons/globe.svg"

// iconExts are extensions we keep from the URL's own filename when caching
// an icon locally.
var iconExts = map[string]string{
	".png": ".png", ".jpg": ".jpg", ".jpeg": ".jpeg",
	".svg": ".svg", ".ico": ".ico", ".gif": ".gif", ".webp": ".webp",
}

// ensureIcons downloads every URL icon once into the local resources
// folder. Files are skipped if already present, so icons keep working
// offline and nothing is re-fetched on every reload. A failed download is
// logged and the item falls back to the placeholder icon at render time.
func (m *Manager) ensureIcons(cfg *domain.Config) {
	iconsDir := filepath.Join(m.resPath, "icons")
	if err := os.MkdirAll(iconsDir, 0o755); err != nil {
		m.logger.Printf("creating icons dir: %v", err)
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	cfg.Walk(func(path string, g *domain.Group) {
		// groups are structural — no icon/host; only children carry icons
		for _, ch := range g.Items {
			icon := ch.Core().Icon
			if !isURL(icon) {
				continue
			}
			dest := filepath.Join(iconsDir, iconFilename(icon))
			if _, err := os.Stat(dest); err == nil {
				continue
			}
			if err := downloadIcon(client, icon, dest); err != nil {
				m.logger.Printf("downloading icon %s for %q: %v", icon, ch.Core().Title, err)
			}
		}
	})
}

// ResolveIcon maps an item's `icon` field to a URL the template can use for
// this render. URL icons point at the locally cached copy; bare names are
// looked up in the same folder (with extension, e.g. `jellyfin.svg`);
// anything unusable falls back to the placeholder.
func (m *Manager) ResolveIcon(icon string) string {
	iconsDir := filepath.Join(m.resPath, "icons")
	switch {
	case isURL(icon):
		return "/resources/icons/" + iconFilename(icon)
	case icon != "":
		local := filepath.Join(iconsDir, icon)
		if _, err := os.Stat(local); err == nil {
			return "/resources/icons/" + icon
		}
	}
	return FallbackIcon
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func iconFilename(rawURL string) string {
	ext := ".png"
	if u, err := parseURLPathExt(rawURL); err == nil && u != "" {
		ext = u
	}
	sum := sha256.Sum256([]byte(rawURL))
	return hex.EncodeToString(sum[:])[:12] + ext
}

func parseURLPathExt(rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("parsing icon url: %w", err)
	}
	seg := ""
	if parts := strings.Split(u.Path, "/"); len(parts) > 0 {
		seg = parts[len(parts)-1]
	}
	if i := strings.LastIndex(seg, "."); i > 0 {
		if mapped, ok := iconExts[strings.ToLower(seg[i:])]; ok {
			return mapped, nil
		}
	}
	return "", nil
}

func downloadIcon(client *http.Client, rawURL, dest string) error {
	resp, err := client.Get(rawURL) //nolint:gosec // icon URLs are authored in the local config file
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %s", resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return nil
}
