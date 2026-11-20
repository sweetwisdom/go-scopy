// Local app snapshots are disposable: never use them to skip device validation.
package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
)

const (
	appCacheIndexLimit = 8 << 20
	appCacheIconLimit  = 4 << 20
	appCacheTotalLimit = 64 << 20
)

type cachedApp struct {
	domain.AppInfo
	Icon string `json:",omitempty"`
}

type appCacheIndex struct {
	Version int
	Apps    []cachedApp
}

func appCacheHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ro.serialno lets USB and wireless transports of the same phone share files.
// A missing/placeholder hardware ID falls back to the exact transport address;
// model names must never be used as identity for different phones.
func (s *Service) appCacheDevice(ctx context.Context, serial string) string {
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	identity := "transport:" + serial
	if out, err := s.tool.Device(readCtx, serial, "shell", "getprop ro.serialno"); err == nil {
		value := strings.TrimSpace(string(out))
		if value != "" && len(value) <= 256 && !strings.ContainsAny(value, "\r\n") &&
			!strings.EqualFold(value, "unknown") && !strings.EqualFold(value, "null") && value != "0" {
			identity = "hardware:" + value
		}
	}
	key := appCacheHash([]byte(identity))
	return key
}

func (s *Service) appCacheDir(key string) string {
	return filepath.Join(s.dir, "cache", "app-icons", key)
}

func readAppCacheFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = errors.New("app cache entry too large")
	}
	return data, err
}

func validAppCacheIcon(data []byte) bool {
	if len(data) == 0 || len(data) > appCacheIconLimit {
		return false
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	return err == nil && cfg.Width > 0 && cfg.Height > 0 && cfg.Width <= 2048 && cfg.Height <= 2048
}

func (s *Service) loadAppCache(key string) ([]domain.AppInfo, error) {
	s.appCacheMu.Lock()
	defer s.appCacheMu.Unlock()
	dir := s.appCacheDir(key)
	data, err := readAppCacheFile(filepath.Join(dir, "apps.json"), appCacheIndexLimit)
	if err != nil {
		return nil, err
	}
	var index appCacheIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	if index.Version != 1 || len(index.Apps) > 10000 {
		return nil, errors.New("invalid app cache index")
	}
	apps := make([]domain.AppInfo, 0, len(index.Apps))
	seen := map[string]bool{}
	icons := map[string][]byte{}
	total := 0
	for _, entry := range index.Apps {
		a := entry.AppInfo
		a.IconPNG = nil // Only content-addressed PNG files are trusted, not inline data.
		if a.Package == "" || seen[a.Package] {
			return nil, errors.New("invalid cached package")
		}
		seen[a.Package] = true
		if len(entry.Icon) == 64 && strings.Trim(entry.Icon, "0123456789abcdef") == "" {
			icon, ok := icons[entry.Icon]
			if !ok && total < appCacheTotalLimit {
				icon, err = readAppCacheFile(filepath.Join(dir, entry.Icon+".png"), appCacheIconLimit)
				if err == nil && appCacheHash(icon) == entry.Icon && validAppCacheIcon(icon) && total+len(icon) <= appCacheTotalLimit {
					total += len(icon)
				} else {
					icon = nil
				}
				icons[entry.Icon] = icon
			}
			a.IconPNG = icon
		}
		apps = append(apps, a)
	}
	return apps, nil
}

// Write complete files before replacing the index. A failed cache write cannot
// destroy the authoritative in-memory listing, and is deliberately nonfatal.
func writeAppCacheFile(path string, data []byte) error {
	if previous, err := readAppCacheFile(path, int64(len(data))); err == nil && bytes.Equal(previous, data) {
		return nil
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".app-cache-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (s *Service) saveAppCache(key string, apps []domain.AppInfo) error {
	s.appCacheMu.Lock()
	defer s.appCacheMu.Unlock()
	if len(apps) > 10000 {
		return errors.New("too many cached apps")
	}
	dir := s.appCacheDir(key)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	index := appCacheIndex{Version: 1, Apps: make([]cachedApp, 0, len(apps))}
	total := 0
	written := map[string]bool{}
	for _, a := range apps {
		entry := cachedApp{AppInfo: a}
		entry.IconPNG = nil
		if validAppCacheIcon(a.IconPNG) {
			hash := appCacheHash(a.IconPNG)
			if !written[hash] && total+len(a.IconPNG) <= appCacheTotalLimit {
				if err := writeAppCacheFile(filepath.Join(dir, hash+".png"), a.IconPNG); err != nil {
					return err
				}
				written[hash] = true
				total += len(a.IconPNG)
			}
			if written[hash] {
				entry.Icon = hash
			}
		}
		index.Apps = append(index.Apps, entry)
	}
	data, err := json.Marshal(index)
	if err != nil {
		return err
	}
	if len(data) > appCacheIndexLimit {
		return errors.New("app cache index too large")
	}
	return writeAppCacheFile(filepath.Join(dir, "apps.json"), data)
}

// The caller holds s.mu. Replace old metadata, including removed applications,
// so a device refresh cannot keep an icon from an older app version alive.
func (s *Service) setAppListing(serial string, listing domain.AppListing) {
	if s.state.Apps == nil {
		s.state.Apps = map[string]domain.AppListing{}
	}
	if s.state.AppMetas == nil {
		s.state.AppMetas = map[string]domain.AppMeta{}
	}
	prefix := serial + "\x00"
	for key := range s.state.AppMetas {
		if strings.HasPrefix(key, prefix) {
			delete(s.state.AppMetas, key)
		}
	}
	for _, a := range listing.Apps {
		if a.Label != "" || len(a.IconPNG) > 0 {
			s.state.AppMetas[prefix+a.Package] = domain.AppMeta{Package: a.Package, Label: a.Label, IconPNG: a.IconPNG}
		}
	}
	s.state.Apps[serial] = listing
}
