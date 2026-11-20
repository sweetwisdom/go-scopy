package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
)

// AppExport preserves the directory-based backend entry point.
func (s *Service) AppExport(serial, pkg, localDir string) {
	if localDir == "" {
		s.mu.Lock()
		localDir = s.state.Settings.OutputDir
		s.mu.Unlock()
	}
	s.ExportApps(serial, []string{pkg}, localDir, false)
}

// ExportApps runs a batch sequentially so file counts and the current transfer stay clear.
// singleFile destinations come from the native SaveFile picker (with overwrite confirmation).
func (s *Service) ExportApps(serial string, packages []string, destination string, singleFile bool) {
	packages = append([]string(nil), packages...)
	progress := domain.AppExportProgress{Serial: serial, Total: len(packages)}
	s.submitWithProgress("导出安装包", progress, func(ctx context.Context, report func(domain.AppExportProgress)) (string, string, error) {
		if len(packages) == 0 || destination == "" || (singleFile && len(packages) != 1) {
			return "", "", errors.New("导出应用和保存位置无效")
		}
		if err := s.online(serial); err != nil {
			return "", "", err
		}
		destination, err := filepath.Abs(destination)
		if err != nil {
			return "", "", err
		}
		dir := destination
		if singleFile {
			dir = filepath.Dir(destination)
		}
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", "", err
		}
		var failures []string
		lastPath := ""
		for _, pkg := range packages {
			if err := ctx.Err(); err != nil {
				return "", lastPath, err
			}
			progress.Current, progress.Bytes, progress.TotalBytes = pkg, 0, 0
			progress.Phase = "准备安装包"
			report(progress)
			info := domain.AppInfo{Package: pkg}
			s.mu.Lock()
			for _, a := range s.state.Apps[serial].Apps {
				if a.Package == pkg {
					info = a
					break
				}
			}
			meta := s.state.AppMetas[serial+"\x00"+pkg]
			s.mu.Unlock()
			target := destination
			if !singleFile {
				target = filepath.Join(dir, domain.AppExportFilename(info, meta))
			}
			local, err := s.exportAPK(ctx, serial, pkg, target, !singleFile, func(bytes, total int64) {
				progress.Bytes, progress.TotalBytes = bytes, total
				progress.Phase = "正在传输"
				report(progress)
			})
			progress.Completed++
			if err != nil {
				progress.Failed++
				failures = append(failures, pkg+"："+err.Error())
			} else {
				lastPath = local
			}
			report(progress)
		}
		detail := fmt.Sprintf("已导出 %d/%d 个应用", progress.Completed-progress.Failed, progress.Total)
		if len(failures) > 0 {
			return "", lastPath, errors.New(detail + "；" + strings.Join(failures, "；"))
		}
		return detail, lastPath, nil
	})
}

var exportPackagePattern = regexp.MustCompile(`^[A-Za-z0-9_]+(?:\.[A-Za-z0-9_]+)*$`)

func (s *Service) exportAPK(parent context.Context, serial, pkg, target string, unique bool, report func(int64, int64)) (string, error) {
	if !exportPackagePattern.MatchString(pkg) {
		return "", errors.New("无效应用包名")
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	paths, err := s.tool.AppPaths(ctx, serial, pkg)
	if err != nil {
		return "", err
	}
	remote := paths[0]
	for _, p := range paths {
		if path.Base(p) == "base.apk" {
			remote = p
			break
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".goscopy-export-*.apk")
	if err != nil {
		return "", err
	}
	tempPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		os.Remove(tempPath)
		return "", err
	}
	defer os.Remove(tempPath)
	if err := s.tool.PullProgress(ctx, serial, remote, tempPath, report); err != nil {
		return "", err
	}
	info, err := os.Stat(tempPath)
	if err != nil {
		return "", err
	}
	if info.Size() == 0 {
		return "", errors.New("设备返回了空安装包")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if unique {
		target, err = reserveExportPath(target)
		if err != nil {
			return "", err
		}
	}
	if err := os.Rename(tempPath, target); err != nil {
		if unique {
			os.Remove(target)
		} // Only the exclusively reserved placeholder is ours.
		return "", err
	}
	return target, nil
}

// Exclusive reservation protects existing files even when two batches finish together.
func reserveExportPath(target string) (string, error) {
	ext := filepath.Ext(target)
	stem := strings.TrimSuffix(target, ext)
	for n := 0; n < 10000; n++ {
		candidate := target
		if n > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, n, ext)
		}
		file, err := os.OpenFile(candidate, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err := file.Close(); err != nil {
			os.Remove(candidate)
			return "", err
		}
		return candidate, nil
	}
	return "", errors.New("同名导出文件过多，请选择其他目录")
}
