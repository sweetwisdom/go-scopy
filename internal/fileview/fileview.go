// Package fileview contains the embedded file workspace and its narrow backend contract.
package fileview

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/go-scopy/go-scopy/internal/domain"
)

// Backend keeps browser content away from local paths, subprocesses and the OS.
// Reads run on workers; mutations remain ordinary task-center operations.
type Backend interface {
	BrowseFiles(context.Context, string, string) ([]domain.FileEntry, error)
	ReadFilePreview(context.Context, string, string, int64) ([]byte, error)
	FileTask(string, Operation) uint64
}

type Operation struct {
	Action    string
	Path      string
	Target    string
	Recursive bool
}

type Spec struct {
	Kind  string `json:"kind"`
	MIME  string `json:"mime"`
	Limit int64  `json:"limit"`
}

func PreviewSpec(name string) (Spec, error) {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".txt", ".md", ".log", ".json", ".xml", ".yml", ".yaml", ".ini", ".conf", ".cfg", ".csv", ".js", ".ts", ".py", ".java", ".c", ".cpp", ".h", ".go", ".rs", ".vue", ".jsx", ".tsx", ".html", ".htm", ".css", ".scss", ".sh", ".sql", ".toml", ".properties", ".prop", ".gradle", ".kt", ".dart", ".ps1", ".bat", ".srt", ".vtt", ".lrc", ".gitignore":
		return Spec{"text", "text/plain; charset=utf-8", 2 << 20}, nil
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".ico":
		return Spec{"image", mime.TypeByExtension(ext), 32 << 20}, nil
	case ".mp4", ".webm", ".mov", ".m4v":
		return Spec{"video", mime.TypeByExtension(ext), 64 << 20}, nil
	case ".mp3", ".wav", ".flac", ".aac", ".ogg", ".m4a", ".opus":
		return Spec{"audio", mime.TypeByExtension(ext), 64 << 20}, nil
	}
	return Spec{}, errors.New("此格式暂不支持内嵌预览，请下载后打开")
}

// ValidatePreview never serves a device HTML document or SVG as active content.
func ValidatePreview(spec Spec, data []byte) error {
	if int64(len(data)) > spec.Limit {
		return fmt.Errorf("文件超过预览上限（%d MB），请下载后打开", spec.Limit>>20)
	}
	if spec.Kind == "text" {
		if !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
			return errors.New("不是 UTF-8 文本，请下载后使用合适的应用打开")
		}
	} else if spec.Kind == "image" && !strings.HasPrefix(http.DetectContentType(data), "image/") {
		return errors.New("文件内容不是可预览的图片")
	}
	return nil
}

func CleanPath(value string) (string, error) {
	if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\x00\r\n") || !utf8.ValidString(value) {
		return "", errors.New("请输入有效的设备绝对路径")
	}
	return path.Clean(value), nil
}

func ValidName(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "/\x00\r\n") && utf8.ValidString(value)
}

//go:embed assets/*
var assets embed.FS

// Resources holds at most one bounded preview. Tokens are invalidated whenever
// a preview closes, navigation changes or the window closes. No temp files or
// local HTTP listener are needed.
type Resources struct {
	mu          sync.RWMutex
	token, mime string
	data        []byte
}

func (r *Resources) Clear() { r.mu.Lock(); r.token, r.mime, r.data = "", "", nil; r.mu.Unlock() }

func (r *Resources) Put(spec Spec, data []byte) (string, error) {
	if err := ValidatePreview(spec, data); err != nil {
		return "", err
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	r.mu.Lock()
	r.token, r.mime, r.data = hex.EncodeToString(token[:]), spec.MIME, data
	url := "./preview/" + r.token
	r.mu.Unlock()
	return url, nil
}

func (r *Resources) Serve(name string) (string, []byte, bool) {
	name = strings.TrimPrefix(name, "/")
	if strings.HasPrefix(name, "preview/") {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if r.token == "" || name != "preview/"+r.token {
			return "", nil, false
		}
		return r.mime, r.data, true
	}
	if name == "" {
		name = "index.html"
	}
	if !fs.ValidPath(name) || strings.Contains(name, "/") {
		return "", nil, false
	}
	data, err := assets.ReadFile("assets/" + name)
	if err != nil {
		return "", nil, false
	}
	return mime.TypeByExtension(path.Ext(name)), data, true
}
