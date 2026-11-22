package core

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/fileview"
)

var _ fileview.Backend = (*Service)(nil)

func (s *Service) fileReadContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	stop := context.AfterFunc(s.ctx, cancel)
	return ctx, func() { stop(); cancel() }
}

func (s *Service) BrowseFiles(ctx context.Context, serial, dir string) ([]domain.FileEntry, error) {
	dir, err := fileview.CleanPath(dir)
	if err != nil {
		return nil, err
	}
	if err := s.online(serial); err != nil {
		return nil, err
	}
	ctx, cancel := s.fileReadContext(ctx)
	defer cancel()
	return s.tool.ListFiles(ctx, serial, dir)
}

func (s *Service) ReadFilePreview(ctx context.Context, serial, remote string, limit int64) ([]byte, error) {
	remote, err := fileview.CleanPath(remote)
	if err != nil {
		return nil, err
	}
	if err := s.online(serial); err != nil {
		return nil, err
	}
	ctx, cancel := s.fileReadContext(ctx)
	defer cancel()
	return s.tool.ReadFilePreview(ctx, serial, remote, limit)
}

// FileTask is shared by the native fallback and web workspace. Local paths
// come only from the native picker, never from a page message.
func (s *Service) FileTask(serial string, op fileview.Operation) uint64 {
	titles := map[string]string{"mkdir": "新建文件夹", "rename": "重命名", "delete": "删除", "upload": "上传", "download": "下载"}
	title := titles[op.Action]
	if title == "" {
		title = "文件操作"
	}
	return s.submit(title+" · "+path.Base(op.Path), func(ctx context.Context) (string, string, error) {
		if err := s.online(serial); err != nil {
			return "", "", err
		}
		remote, err := fileview.CleanPath(op.Path)
		if err != nil {
			return "", "", err
		}
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		switch op.Action {
		case "mkdir":
			err = s.tool.MakeDir(ctx, serial, remote)
		case "rename":
			target, e := fileview.CleanPath(op.Target)
			if e != nil {
				return "", "", e
			}
			if remote == "/" || target == "/" {
				return "", "", errors.New("不允许重命名设备根目录")
			}
			err = s.tool.RenamePath(ctx, serial, remote, target)
		case "delete":
			err = s.tool.RemovePath(ctx, serial, remote, op.Recursive)
		case "upload":
			if op.Target == "" {
				return "", "", errors.New("未选择本地文件")
			}
			info, statErr := os.Stat(op.Target)
			if statErr != nil {
				return "", "", statErr
			}
			destination := remote
			if info.IsDir() {
				// Pushing to the parent preserves the selected folder exactly once,
				// including when an earlier upload already created it.
				destination = path.Dir(remote)
				if err = s.tool.MakeDir(ctx, serial, destination); err != nil {
					return "", "", err
				}
			}
			_, err = s.tool.Push(ctx, serial, op.Target, destination)
		case "download":
			target := op.Target
			if target == "" {
				s.mu.Lock()
				target = s.state.Settings.OutputDir
				s.mu.Unlock()
			}
			if err := os.MkdirAll(target, 0700); err != nil {
				return "", "", err
			}
			name := path.Base(remote)
			if !filepath.IsLocal(name) || strings.ContainsAny(name, `/\:`) {
				return "", "", errors.New("设备文件名不能安全保存到本机，请先在设备上重命名")
			}
			destination := filepath.Join(target, name)
			if _, err := os.Lstat(destination); err == nil {
				return "", "", errors.New("下载目标已有同名项目，请选择其他目录以避免覆盖")
			} else if !os.IsNotExist(err) {
				return "", "", err
			}
			out, err := s.tool.Pull(ctx, serial, remote, target)
			return out, destination, err
		default:
			return "", "", errors.New("不支持的文件操作")
		}
		if err != nil {
			return "", "", err
		}
		s.FetchFiles(serial, parentDir(remote))
		return title + "完成", "", nil
	})
}
