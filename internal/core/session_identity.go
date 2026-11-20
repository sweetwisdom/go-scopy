package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-scopy/go-scopy/internal/brand"
	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/tools"
)

func (s *Service) sessionIdentity(ctx context.Context, serial string, display *domain.VirtualDisplay) (tools.WindowIdentity, func(), error) {
	identity := tools.WindowIdentity{Title: "Go-Scopy · " + serial}
	icon := brand.PNG()
	if display != nil && display.StartApp != "" {
		pkg := display.StartApp
		s.mu.Lock()
		meta := s.state.SharedAppMetas[pkg]
		if meta.Label == "" {
			meta = s.state.AppMetas[serial+"\x00"+pkg]
		}
		apkPath := ""
		for _, a := range s.state.Apps[serial].Apps {
			if a.Package == pkg {
				apkPath = a.APKPath
				if meta.Label == "" {
					meta.Label = a.Label
				}
				if len(meta.IconPNG) == 0 {
					meta.IconPNG = a.IconPNG
				}
				break
			}
		}
		s.mu.Unlock()
		if meta.Label == "" || len(meta.IconPNG) == 0 {
			readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			resolved := s.resolveAppMeta(readCtx, serial, apkPath, pkg)
			cancel()
			if resolved.Label != "" {
				meta.Label = resolved.Label
			}
			if validAppCacheIcon(resolved.IconPNG) {
				meta.IconPNG = resolved.IconPNG
			}
		}
		identity.Title = strings.TrimSpace(meta.Label)
		if identity.Title == "" {
			identity.Title = pkg
		}
		if validAppCacheIcon(meta.IconPNG) {
			icon = meta.IconPNG
		}
	}
	if err := ctx.Err(); err != nil {
		return identity, func() {}, err
	}
	root := filepath.Join(s.dir, "cache", "session-icons")
	if err := os.MkdirAll(root, 0700); err != nil {
		return identity, func() {}, err
	}
	dir, err := os.MkdirTemp(root, "session-")
	if err != nil {
		return identity, func() {}, err
	}
	// Do not recursively remove computed paths: only known files in our own
	// newly created directory. No shared cache file belongs to this cleanup.
	cleanup := func() {
		_ = os.Remove(filepath.Join(dir, "scrcpy.png"))
		_ = os.Remove(filepath.Join(dir, "disconnected.png"))
		_ = os.Remove(dir)
	}
	if err := os.WriteFile(filepath.Join(dir, "scrcpy.png"), icon, 0600); err != nil {
		cleanup()
		return identity, func() {}, err
	}
	disconnected := brand.PNG()
	if data, err := os.ReadFile(filepath.Join(filepath.Dir(s.tool.Scrcpy), "disconnected.png")); err == nil && validAppCacheIcon(data) {
		disconnected = data
	}
	if err := os.WriteFile(filepath.Join(dir, "disconnected.png"), disconnected, 0600); err != nil {
		cleanup()
		return identity, func() {}, err
	}
	identity.IconDir = dir
	return identity, cleanup, nil
}
