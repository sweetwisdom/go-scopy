package core

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
)

type globalAppMeta struct {
	Version              int
	Package, Label, Icon string
}

type appAgentBatch struct {
	done chan struct{}
	apps map[string]domain.AppInfo
	err  error
}

func validCacheHash(key string) bool {
	return len(key) == 64 && strings.Trim(key, "0123456789abcdef") == ""
}
func (s *Service) globalAppsDir() string { return filepath.Join(s.dir, "cache", "apps-v2") }

func (s *Service) loadGlobalAppMeta(key, pkg string) (domain.AppMeta, error) {
	s.appCacheMu.Lock()
	defer s.appCacheMu.Unlock()
	meta := domain.AppMeta{Package: pkg, Resolved: true}
	if !validCacheHash(key) {
		return meta, errors.New("invalid app identity")
	}
	path := filepath.Join(s.globalAppsDir(), "metadata", key+".json")
	data, err := readAppCacheFile(path, 16<<10)
	if err != nil {
		return meta, err
	}
	var entry globalAppMeta
	if json.Unmarshal(data, &entry) != nil || entry.Version != 2 || entry.Package != pkg || entry.Label == "" {
		return meta, errors.New("invalid app metadata")
	}
	meta.Label = entry.Label
	if !validCacheHash(entry.Icon) {
		return meta, errors.New("cached app icon missing")
	}
	if validCacheHash(entry.Icon) {
		iconPath := filepath.Join(s.globalAppsDir(), "icons", entry.Icon+".png")
		icon, err := readAppCacheFile(iconPath, appCacheIconLimit)
		if err != nil || appCacheHash(icon) != entry.Icon || !validAppCacheIcon(icon) {
			return meta, errors.New("invalid cached icon")
		}
		meta.IconPNG = icon
		now := time.Now()
		_ = os.Chtimes(iconPath, now, now)
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now)
	return meta, nil
}

func (s *Service) saveGlobalAppMeta(key string, meta domain.AppMeta) error {
	if !validCacheHash(key) || meta.Package == "" || meta.Label == "" || meta.Error != "" {
		return nil
	}
	s.appCacheMu.Lock()
	defer s.appCacheMu.Unlock()
	return s.saveGlobalAppMetaLocked(key, meta)
}

func (s *Service) saveGlobalAppMetaLocked(key string, meta domain.AppMeta) error {
	dir := s.globalAppsDir()
	for _, sub := range []string{"metadata", "icons"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0700); err != nil {
			return err
		}
	}
	entry := globalAppMeta{Version: 2, Package: meta.Package, Label: meta.Label}
	if validAppCacheIcon(meta.IconPNG) {
		entry.Icon = appCacheHash(meta.IconPNG)
		if err := writeAppCacheFile(filepath.Join(dir, "icons", entry.Icon+".png"), meta.IconPNG); err != nil {
			return err
		}
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if err := writeAppCacheFile(filepath.Join(dir, "metadata", key+".json"), data); err != nil {
		return err
	}
	s.appCacheWrites++
	// Amortize directory scans; at most 15 new entries exceed the soft limits.
	if s.appCacheWrites == 1 || s.appCacheWrites%16 == 0 {
		pruneAppCacheDir(filepath.Join(dir, "metadata"), ".json", 2048, 8<<20)
		pruneAppCacheDir(filepath.Join(dir, "icons"), ".png", 2048, appCacheTotalLimit)
	}
	return nil
}

// Only remove hash-named regular files in the explicitly owned directory.
func pruneAppCacheDir(dir, extension string, countLimit int, sizeLimit int64) {
	entries, _ := os.ReadDir(dir)
	var files []os.FileInfo
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() || !strings.HasSuffix(entry.Name(), extension) || !validCacheHash(strings.TrimSuffix(entry.Name(), extension)) {
			continue
		}
		if info, err := entry.Info(); err == nil && info.Mode().IsRegular() {
			files = append(files, info)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ModTime().After(files[j].ModTime()) })
	var total int64
	for i, file := range files {
		total += file.Size()
		if i >= countLimit || total > sizeLimit {
			_ = os.Remove(filepath.Join(dir, file.Name()))
		}
	}
}

// A package is intentionally the sole shared identity. Different phones and
// transports reuse it immediately; users can clear the cache after an app update.
func appPackageKey(pkg string) string { return appCacheHash([]byte("package:" + pkg)) }

func completeAppMeta(meta domain.AppMeta) bool {
	return meta.Label != "" && len(meta.IconPNG) > 0 && meta.Error == ""
}

// Restore the entire listing's shared identities before deciding whether a
// device read is necessary. A warm list never starts the Agent merely because
// pm list packages does not return labels. Disk hits are not written back.
func (s *Service) hydrateAppMetadata(epoch uint64, apps []domain.AppInfo) bool {
	var loaded []domain.AppMeta
	missing := false
	for _, app := range apps {
		if epoch != s.appCacheEpoch.Load() {
			return false
		}
		s.mu.Lock()
		meta := s.state.SharedAppMetas[app.Package]
		s.mu.Unlock()
		if completeAppMeta(meta) {
			continue
		}
		if meta, err := s.loadGlobalAppMeta(appPackageKey(app.Package), app.Package); err == nil {
			loaded = append(loaded, meta)
		} else {
			missing = true
		}
	}
	s.cacheSharedAppMetas(epoch, loaded, false)
	return missing
}

func (s *Service) resolveAppMeta(ctx context.Context, serial, apkPath, pkg string) domain.AppMeta {
	epoch := s.appCacheEpoch.Load()
	s.mu.Lock()
	cached, ok := s.state.SharedAppMetas[pkg]
	s.mu.Unlock()
	if ok && completeAppMeta(cached) {
		return cached
	}
	if cached, err := s.loadGlobalAppMeta(appPackageKey(pkg), pkg); err == nil {
		s.cacheSharedAppMetas(epoch, []domain.AppMeta{cached}, false)
		return cached
	}
	meta := domain.AppMeta{Package: pkg, Resolved: true}
	s.mu.Lock()
	if s.appMetaSlots == nil {
		s.appMetaSlots = make(chan struct{}, 4)
	}
	slots := s.appMetaSlots
	s.mu.Unlock()
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		meta.Error = ctx.Err().Error()
		return meta
	}
	defer func() { <-slots }()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	// Another transport may have filled the shared entry while this read queued.
	s.mu.Lock()
	cached, ok = s.state.SharedAppMetas[pkg]
	s.mu.Unlock()
	if ok && completeAppMeta(cached) {
		return cached
	}
	// Android PackageManager understands adaptive icons and modern resources.
	// Do not run the legacy APK parser in the desktop process: malformed or
	// unsupported resource tables can panic or allocate unbounded memory.
	if app, ok := s.agentAppMeta(ctx, serial, pkg); ok {
		meta.Label, meta.IconPNG = app.Label, app.IconPNG
	}
	if err := ctx.Err(); err != nil {
		meta.Error = err.Error()
		return meta
	}
	if !validAppCacheIcon(meta.IconPNG) {
		meta.IconPNG = nil
		if meta.Label == "" {
			meta.Label = pkg
		}
		meta.Error = "应用图标读取失败，可刷新列表重试"
		return meta
	}
	if meta.Label == "" {
		meta.Label = pkg
	}
	s.publishSharedAppMetas(epoch, []domain.AppMeta{meta})
	return meta
}

// Lock order is cache -> state, also used by explicit clearing. The epoch fences
// reads already running when the user clears, so old results cannot repopulate it.
func (s *Service) publishSharedAppMetas(epoch uint64, metas []domain.AppMeta) {
	s.cacheSharedAppMetas(epoch, metas, true)
}

func (s *Service) cacheSharedAppMetas(epoch uint64, metas []domain.AppMeta, persist bool) {
	s.appCacheMu.Lock()
	defer s.appCacheMu.Unlock()
	if epoch != s.appCacheEpoch.Load() {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if s.state.SharedAppMetas == nil {
		s.state.SharedAppMetas = map[string]domain.AppMeta{}
	}
	var accepted []domain.AppMeta
	for _, meta := range metas {
		if meta.Package == "" || meta.Label == "" || meta.Error != "" {
			continue
		}
		meta.Resolved = true
		if !validAppCacheIcon(meta.IconPNG) {
			meta.IconPNG = nil
		}
		if old := s.state.SharedAppMetas[meta.Package]; len(meta.IconPNG) == 0 && len(old.IconPNG) > 0 {
			meta.IconPNG = old.IconPNG
		}
		// Label-only results must never mask an unresolved icon across devices.
		if len(meta.IconPNG) == 0 {
			continue
		}
		if old := s.state.SharedAppMetas[meta.Package]; completeAppMeta(old) {
			// The explicitly chosen package-only cache remains authoritative until
			// cleared; enrichment for another missing app must not replace it.
			continue
		}
		s.sharedAppBytes -= len(s.state.SharedAppMetas[meta.Package].IconPNG)
		s.state.SharedAppMetas[meta.Package] = meta
		s.sharedAppBytes += len(meta.IconPNG)
		for len(s.state.SharedAppMetas) > 2048 || s.sharedAppBytes > appCacheTotalLimit {
			for pkg, old := range s.state.SharedAppMetas {
				if pkg != meta.Package {
					s.sharedAppBytes -= len(old.IconPNG)
					delete(s.state.SharedAppMetas, pkg)
					break
				}
			}
		}
		accepted = append(accepted, meta)
	}
	s.mu.Unlock()
	if persist {
		for _, meta := range accepted {
			if err := s.saveGlobalAppMetaLocked(appPackageKey(meta.Package), meta); err != nil {
				log.Printf("save app icon cache %s: %v", meta.Package, err)
			}
		}
	}
	if len(accepted) > 0 {
		s.wake()
	}
}

func (s *Service) agentAppMeta(ctx context.Context, serial, pkg string) (domain.AppInfo, bool) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return domain.AppInfo{}, false
	}
	if s.agentBatches == nil {
		s.agentBatches = map[string]*appAgentBatch{}
	}
	batch := s.agentBatches[serial]
	if batch == nil {
		batch = &appAgentBatch{done: make(chan struct{})}
		s.agentBatches[serial] = batch
		s.wg.Add(1)
		generation := s.appListingGeneration[serial]
		epoch := s.appCacheEpoch.Load()
		go func() {
			defer s.wg.Done()
			defer close(batch.done)
			readCtx, cancel := context.WithTimeout(s.ctx, 45*time.Second)
			defer cancel()
			apps, err := s.tool.ListAppMetadata(readCtx, serial)
			batch.err = err
			if err != nil {
				log.Printf("read app icons for %s: %v", serial, err)
				return
			}
			batch.apps = make(map[string]domain.AppInfo, len(apps))
			for _, a := range apps {
				batch.apps[a.Package] = a
			}
			s.publishAgentApps(serial, generation, epoch, batch.apps)
		}()
	}
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		return domain.AppInfo{}, false
	case <-batch.done:
		if batch.err != nil {
			return domain.AppInfo{}, false
		}
		app, ok := batch.apps[pkg]
		return app, ok
	}
}

// Publish the second phase without replacing authoritative installation paths.
// PNGs are transient here; persisted snapshots contain only device metadata,
// while package-keyed names and content-addressed icons belong to the global store.
func (s *Service) publishAgentApps(serial string, generation, epoch uint64, apps map[string]domain.AppInfo) {
	if len(apps) == 0 {
		return
	}
	// Global identities are package-scoped even if the installation listing was
	// refreshed while the Agent was reading. The epoch still fences clearing.
	var shared []domain.AppMeta
	for _, a := range apps {
		if a.Label != "" {
			shared = append(shared, domain.AppMeta{Package: a.Package, Label: a.Label, IconPNG: a.IconPNG, Resolved: true})
		}
	}
	s.publishSharedAppMetas(epoch, shared)
	s.mu.Lock()
	if s.closed || s.appListingGeneration[serial] != generation || s.appCacheEpoch.Load() != epoch {
		s.mu.Unlock()
		return
	}
	listing, exists := s.state.Apps[serial]
	if !exists {
		s.mu.Unlock()
		return
	}
	listing.Apps = append([]domain.AppInfo(nil), listing.Apps...)
	total := 0
	for i, a := range listing.Apps {
		meta, ok := apps[a.Package]
		if !ok || (a.VersionCode != "" && a.VersionCode != meta.VersionCode) {
			continue
		}
		a.Label, a.VersionName, a.SizeBytes, a.CanLaunch = meta.Label, meta.VersionName, meta.SizeBytes, meta.CanLaunch
		if a.VersionCode == "" {
			a.VersionCode = meta.VersionCode
		}
		if validAppCacheIcon(meta.IconPNG) && total+len(meta.IconPNG) <= appCacheTotalLimit {
			a.IconPNG = meta.IconPNG
			total += len(meta.IconPNG)
		}
		listing.Apps[i] = a
	}
	s.state.Apps[serial] = listing
	key := s.appCacheKeys[serial]
	s.mu.Unlock()
	s.wake()
	if key != "" {
		_ = s.saveAppSnapshot(epoch, key, appSnapshot(listing.Apps))
	}
}

func appSnapshot(apps []domain.AppInfo) []domain.AppInfo {
	snapshot := append([]domain.AppInfo(nil), apps...)
	for i := range snapshot {
		snapshot[i].IconPNG = nil
	}
	return snapshot
}
