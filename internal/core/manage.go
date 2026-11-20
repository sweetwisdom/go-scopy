package core

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/fileview"
)

// fetch runs one management read in the background, coalescing repeats of the
// same key. Results land in State; the UI is woken when they arrive.
func (s *Service) fetch(key string, work func(ctx context.Context)) {
	s.mu.Lock()
	if s.inflight == nil {
		s.inflight = map[string]bool{}
	}
	if s.inflight[key] {
		s.mu.Unlock()
		return
	}
	s.inflight[key] = true
	s.mu.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			delete(s.inflight, key)
			s.mu.Unlock()
		}()
		work(s.ctx)
	}()
}

func (s *Service) FetchPhoneInfo(serial string) {
	s.fetch("info:"+serial, func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		info, err := s.tool.PhoneInfo(ctx, serial)
		if err != nil {
			info.Error = err.Error()
		}
		s.mu.Lock()
		if s.state.PhoneInfos == nil {
			s.state.PhoneInfos = map[string]domain.PhoneInfo{}
		}
		s.state.PhoneInfos[serial] = info
		s.mu.Unlock()
		s.wake()
	})
}

func (s *Service) FetchFiles(serial, dir string) {
	if strings.TrimSpace(dir) == "" {
		dir = "/"
	}
	s.fetch("files:"+serial+"\x00"+dir, func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		listing := domain.FileListing{Path: dir}
		entries, err := s.tool.ListFiles(ctx, serial, dir)
		if err != nil {
			listing.Error = err.Error()
		} else {
			listing.Entries = entries
		}
		s.mu.Lock()
		if s.state.Files == nil {
			s.state.Files = map[string]domain.FileListing{}
		}
		s.state.Files[serial+"\x00"+dir] = listing
		s.mu.Unlock()
		s.wake()
	})
}

func (s *Service) FetchApps(serial string) {
	s.fetch("apps:"+serial, func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		listing := domain.AppListing{}
		apps, err := s.tool.ListApps(ctx, serial)
		if err != nil {
			listing.Error = err.Error()
		} else {
			listing.Apps = apps
		}
		s.mu.Lock()
		if err == nil {
			if s.state.AppMetas == nil {
				s.state.AppMetas = map[string]domain.AppMeta{}
			}
			for _, app := range apps {
				if app.Label != "" || len(app.IconPNG) > 0 {
					s.state.AppMetas[serial+"\x00"+app.Package] = domain.AppMeta{Package: app.Package, Label: app.Label, IconPNG: app.IconPNG}
				}
			}
		}
		if s.state.Apps == nil {
			s.state.Apps = map[string]domain.AppListing{}
		}
		s.state.Apps[serial] = listing
		s.mu.Unlock()
		s.wake()
	})
}

// FetchAppMeta resolves one row's label and icon. The UI asks for the rows it
// shows; the fetch is coalesced so scrolling cannot multiply the work.
func (s *Service) FetchAppMeta(serial, apkPath, pkg string) {
	s.fetch("meta:"+serial+"\x00"+pkg, func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		meta := domain.AppMeta{Package: pkg}
		label, icon, err := s.tool.AppMeta(ctx, serial, apkPath)
		if err != nil {
			meta.Error = err.Error()
		}
		meta.Label, meta.IconPNG = label, icon
		s.mu.Lock()
		if s.state.AppMetas == nil {
			s.state.AppMetas = map[string]domain.AppMeta{}
		}
		s.state.AppMetas[serial+"\x00"+pkg] = meta
		s.mu.Unlock()
		s.wake()
	})
}

func (s *Service) QuickAction(serial, action string) {
	s.submit("快捷操作 · "+serial, func(ctx context.Context) (string, string, error) {
		if err := s.online(serial); err != nil {
			return "", "", err
		}
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		out, err := s.tool.QuickAction(ctx, serial, action)
		return out, "", err
	})
}

func (s *Service) FileMkdir(serial, remote string) {
	s.FileTask(serial, fileview.Operation{Action: "mkdir", Path: remote})
}

func (s *Service) FileRename(serial, from, to string) {
	s.FileTask(serial, fileview.Operation{Action: "rename", Path: from, Target: to})
}

func (s *Service) FileRemove(serial, remote string, recursive bool) {
	s.FileTask(serial, fileview.Operation{Action: "delete", Path: remote, Recursive: recursive})
}

func (s *Service) FileUpload(serial, local, remote string) {
	s.FileTask(serial, fileview.Operation{Action: "upload", Path: remote, Target: local})
}

func (s *Service) FileDownload(serial, remote, localDir string) {
	s.FileTask(serial, fileview.Operation{Action: "download", Path: remote, Target: localDir})
}

// AppExport saves one reinstallable base APK under its package and version
// name.
func (s *Service) AppExport(serial, pkg, localDir string) {
	s.submit("导出安装包 · "+pkg, func(ctx context.Context) (string, string, error) {
		if err := s.online(serial); err != nil {
			return "", "", err
		}
		paths, err := s.tool.AppPaths(ctx, serial, pkg)
		if err != nil {
			return "", "", err
		}
		if localDir == "" {
			s.mu.Lock()
			localDir = s.state.Settings.OutputDir
			s.mu.Unlock()
		}
		if err := os.MkdirAll(localDir, 0700); err != nil {
			return "", "", err
		}
		version := ""
		if apps, err := s.tool.ListApps(ctx, serial); err == nil {
			for _, a := range apps {
				if a.Package == pkg {
					version = a.VersionName
					break
				}
			}
		}
		name := pkg
		if version != "" {
			name += "-" + version
		}
		local := filepath.Join(localDir, name+".apk")
		if _, err := s.tool.Pull(ctx, serial, paths[0], local); err != nil {
			return "", "", err
		}
		return "安装包已导出", local, nil
	})
}

// AppBackup keeps every APK of a package (base plus splits) plus a manifest
// with its package name and version, so the set stays reinstallable.
func (s *Service) AppBackup(serial, pkg, localDir string) {
	s.submit("备份应用 · "+pkg, func(ctx context.Context) (string, string, error) {
		if err := s.online(serial); err != nil {
			return "", "", err
		}
		paths, err := s.tool.AppPaths(ctx, serial, pkg)
		if err != nil {
			return "", "", err
		}
		if localDir == "" {
			s.mu.Lock()
			localDir = s.state.Settings.OutputDir
			s.mu.Unlock()
		}
		target := filepath.Join(localDir, "apps", pkg)
		if err := os.MkdirAll(target, 0700); err != nil {
			return "", "", err
		}
		version := ""
		if apps, err := s.tool.ListApps(ctx, serial); err == nil {
			for _, a := range apps {
				if a.Package == pkg {
					version = a.VersionName
					break
				}
			}
		}
		for _, remote := range paths {
			if _, err := s.tool.Pull(ctx, serial, remote, target); err != nil {
				return "", target, err
			}
		}
		manifest := "package: " + pkg + "\nversion: " + version + "\napks: " + strconv.Itoa(len(paths)) + "\n"
		_ = os.WriteFile(filepath.Join(target, "manifest.txt"), []byte(manifest), 0600)
		return "应用已备份（含 " + strconv.Itoa(len(paths)) + " 个 APK 与清单）", target, nil
	})
}

// AppUninstall removes a user app, or the updates of a system app for the
// current user, then refreshes the app list.
func (s *Service) AppUninstall(serial, pkg string, system bool) {
	s.submit("卸载应用 · "+pkg, func(ctx context.Context) (string, string, error) {
		if err := s.online(serial); err != nil {
			return "", "", err
		}
		out, err := s.tool.Uninstall(ctx, serial, pkg, system)
		s.FetchApps(serial)
		return out, "", err
	})
}

func (s *Service) OpenShell(serial string, cols, rows int, notify func()) (domain.Terminal, error) {
	if err := s.online(serial); err != nil {
		return nil, err
	}
	stream, err := s.tool.DeviceShell(serial, cols, rows, notify)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

func (s *Service) OpenLogcat(serial string, cols, rows int, notify func()) (domain.Terminal, error) {
	if err := s.online(serial); err != nil {
		return nil, err
	}
	stream, err := s.tool.DeviceLogcat(serial, cols, rows, notify)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

// parentDir walks up one device path. Device paths are always
// slash-separated regardless of host platform.
func parentDir(p string) string {
	i := strings.LastIndex(p, "/")
	if i <= 0 {
		return "/"
	}
	return p[:i]
}
