package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-scopy/go-scopy/internal/domain"
)

func (s *Service) ClearAppCache() {
	s.submit("清理应用缓存", func(_ context.Context) (string, string, error) {
		count, err := s.clearAppCache()
		return fmt.Sprintf("已清理 %d 个应用缓存文件；应用名称和图标将在下次打开列表时重新读取", count), "", err
	})
}

func (s *Service) clearAppCache() (int, error) {
	s.appCacheMu.Lock()
	defer s.appCacheMu.Unlock()
	s.appCacheEpoch.Add(1)
	s.mu.Lock()
	s.state.SharedAppMetas = nil
	s.state.AppMetas = nil
	s.sharedAppBytes = 0
	s.agentBatches = nil
	if s.appListingGeneration == nil {
		s.appListingGeneration = map[string]uint64{}
	}
	for serial, listing := range s.state.Apps {
		listing.Apps = append([]domain.AppInfo(nil), listing.Apps...)
		listing.Refreshing = false
		for i := range listing.Apps {
			listing.Apps[i].Label = ""
			listing.Apps[i].IconPNG = nil
		}
		s.state.Apps[serial] = listing
		s.appListingGeneration[serial]++
	}
	s.mu.Unlock()
	count := 0
	var failures []error
	remove := func(path string) {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			failures = append(failures, err)
			return
		}
		if !info.Mode().IsRegular() {
			return
		}
		if err = os.Remove(path); err != nil {
			failures = append(failures, err)
		} else {
			count++
		}
	}
	// Delete only our known regular cache files, never recurse into directories
	// or follow symlinks/junctions. Config, scripts, exports and session icons stay.
	for _, sub := range []struct{ dir, ext string }{{"metadata", ".json"}, {"icons", ".png"}} {
		dir := filepath.Join(s.globalAppsDir(), sub.dir)
		if err := s.ownedAppCacheDir(dir); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				failures = append(failures, err)
			}
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasSuffix(name, sub.ext) && validCacheHash(strings.TrimSuffix(name, sub.ext)) {
				remove(filepath.Join(dir, name))
			}
		}
	}
	legacy := filepath.Join(s.dir, "cache", "app-icons")
	if err := s.ownedAppCacheDir(legacy); err == nil {
		devices, err := os.ReadDir(legacy)
		if err != nil {
			failures = append(failures, err)
		}
		for _, device := range devices {
			if !device.IsDir() || device.Type()&os.ModeSymlink != 0 || !validCacheHash(device.Name()) {
				continue
			}
			dir := filepath.Join(legacy, device.Name())
			entries, err := os.ReadDir(dir)
			if err != nil {
				failures = append(failures, err)
				continue
			}
			for _, entry := range entries {
				name := entry.Name()
				if name == "apps.json" || (strings.HasSuffix(name, ".png") && validCacheHash(strings.TrimSuffix(name, ".png"))) {
					remove(filepath.Join(dir, name))
				}
			}
		}
	}
	s.wake()
	return count, errors.Join(failures...)
}

// Check every directory under the explicitly owned cache root. Checking only
// the final directory would still follow a junction in an ancestor.
func (s *Service) ownedAppCacheDir(dir string) error {
	root, err := filepath.Abs(filepath.Join(s.dir, "cache"))
	if err != nil {
		return err
	}
	target, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("cache path outside owned root")
	}
	current := root
	parts := strings.Split(rel, string(os.PathSeparator))
	for i := -1; i < len(parts); i++ {
		if i >= 0 {
			current = filepath.Join(current, parts[i])
		}
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing linked or non-directory cache path: %s", current)
		}
	}
	return nil
}
