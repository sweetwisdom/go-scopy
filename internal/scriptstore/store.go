// Package scriptstore manages UTF-8 Python files, not a browser-only notebook.
package scriptstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

const MaxSize = 1 << 20

type File struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	External bool   `json:"external"`
}
type Document struct {
	File
	Content string `json:"content"`
	Version string `json:"version"`
	Trusted bool   `json:"trusted"`
}
type Listing struct {
	Directory string `json:"directory"`
	Files     []File `json:"files"`
}
type Store struct {
	mu             sync.Mutex
	root, registry string
	opened         []string
	trusted        map[string]string
}

func New(dataDir string) (*Store, error) {
	root, err := filepath.Abs(filepath.Join(dataDir, "scripts"))
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	s := &Store{root: root, registry: filepath.Join(root, ".opened.json")}
	b, err := os.ReadFile(s.registry)
	if err == nil {
		err = json.Unmarshal(b, &s.opened)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("读取脚本列表：%w", err)
	}
	// Corrupt/missing trust metadata fails closed, without hiding user files.
	s.trusted = map[string]string{}
	if data, e := os.ReadFile(filepath.Join(root, ".trusted.json")); e == nil {
		if json.Unmarshal(data, &s.trusted) != nil || s.trusted == nil {
			s.trusted = map[string]string{}
		}
	}
	return s, nil
}
func (s *Store) managed(path string) bool { return samePath(filepath.Dir(path), s.root) }
func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b) || (os.PathSeparator == '\\' && strings.EqualFold(filepath.Clean(a), filepath.Clean(b)))
}
func (s *Store) allowed(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	if s.managed(path) {
		return true
	}
	for _, p := range s.opened {
		if samePath(p, path) {
			return true
		}
	}
	return false
}
func read(path string) (Document, error) {
	if !strings.EqualFold(filepath.Ext(path), ".py") {
		return Document{}, errors.New("请选择 .py 文件")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return Document{}, err
	}
	if !info.Mode().IsRegular() {
		return Document{}, errors.New("脚本必须为普通文件，不支持符号链接")
	}
	if info.Size() > MaxSize {
		return Document{}, errors.New("脚本不能超过 1 MiB")
	}
	f, err := os.Open(path)
	if err != nil {
		return Document{}, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxSize+1))
	if err != nil {
		return Document{}, err
	}
	if len(b) > MaxSize || !utf8.Valid(b) || strings.IndexByte(string(b), 0) >= 0 {
		return Document{}, errors.New("脚本须为 1 MiB 以内的 UTF-8 文本")
	}
	hash := sha256.Sum256(b)
	return Document{File: File{Path: path, Name: filepath.Base(path)}, Content: string(b), Version: hex.EncodeToString(hash[:])}, nil
}
func (s *Store) List() (Listing, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := Listing{Directory: s.root, Files: []File{}}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return list, err
	}
	for _, e := range entries {
		if e.Type().IsRegular() && strings.EqualFold(filepath.Ext(e.Name()), ".py") {
			list.Files = append(list.Files, File{Path: filepath.Join(s.root, e.Name()), Name: e.Name()})
		}
	}
	for _, p := range s.opened {
		if filepath.IsAbs(p) && !s.managed(p) {
			list.Files = append(list.Files, File{Path: p, Name: filepath.Base(p), External: true})
		}
	}
	sort.Slice(list.Files, func(i, j int) bool { return strings.ToLower(list.Files[i].Name) < strings.ToLower(list.Files[j].Name) })
	return list, nil
}

// Open is called only after the native file picker authorizes an external path.
func (s *Store) Open(path string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := filepath.Abs(path)
	if err != nil {
		return Document{}, err
	}
	d, err := read(path)
	if err != nil {
		return d, err
	}
	d.External = !s.managed(path)
	d.Trusted = s.trusted[trustPath(path)] == d.Version && d.Version != ""
	if !s.allowed(path) {
		if len(s.opened) >= 100 {
			return d, errors.New("外部脚本列表已达 100 项，请将脚本放入默认脚本目录")
		}
		paths := append(append([]string(nil), s.opened...), path)
		b, _ := json.MarshalIndent(paths, "", "  ")
		if err = replace(s.registry, b); err != nil {
			return d, err
		}
		s.opened = paths
	}
	return d, nil
}
func (s *Store) Read(path string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path = filepath.Clean(path)
	if !s.allowed(path) {
		return Document{}, errors.New("请先从脚本列表或文件选择器打开此文件")
	}
	d, err := read(path)
	d.External = !s.managed(path)
	d.Trusted = s.trusted[trustPath(path)] == d.Version && d.Version != ""
	return d, err
}
func validContent(text string) error {
	if len(text) > MaxSize || !utf8.ValidString(text) || strings.IndexByte(text, 0) >= 0 {
		return errors.New("脚本须为 1 MiB 以内的 UTF-8 文本")
	}
	return nil
}
func (s *Store) Create(name, content string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 180 || strings.ContainsAny(name, `/\:*?"<>|`) || strings.IndexFunc(name, unicode.IsControl) >= 0 || strings.HasSuffix(name, ".") || !strings.EqualFold(filepath.Ext(name), ".py") {
		return Document{}, errors.New("请输入有效的 .py 文件名，不含目录或特殊字符")
	}
	if err := validContent(content); err != nil {
		return Document{}, err
	}
	path := filepath.Join(s.root, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return Document{}, fmt.Errorf("创建脚本（不会覆盖已有文件）：%w", err)
	}
	_, err = f.WriteString(content)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(path)
		return Document{}, err
	}
	d, err := read(path)
	d.Trusted = s.trusted[trustPath(path)] == d.Version && d.Version != ""
	return d, err
}
func (s *Store) Save(path, content, version string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path = filepath.Clean(path)
	if !s.allowed(path) {
		return Document{}, errors.New("请先打开此脚本")
	}
	current, err := read(path)
	if err != nil {
		return Document{}, err
	}
	if version == "" || current.Version != version {
		return Document{}, errors.New("文件已被外部修改，请先备份编辑内容并重新打开，未覆盖磁盘文件")
	}
	if err = validContent(content); err != nil {
		return Document{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Document{}, err
	}
	if err = replaceMode(path, []byte(content), info.Mode().Perm()); err != nil {
		return Document{}, err
	}
	d, err := read(path)
	d.External = !s.managed(path)
	d.Trusted = s.trusted[trustPath(path)] == d.Version && d.Version != ""
	return d, err
}
func replace(path string, b []byte) error { return replaceMode(path, b, 0600) }
func replaceMode(path string, b []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".script-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Guide is application-managed help, separate from user-editable scripts.
func (s *Store) Guide(content []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.root, ".help")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "Python脚本开发指南.md")
	return path, replace(path, content)
}

func trustPath(path string) string {
	path = filepath.Clean(path)
	if os.PathSeparator == '\\' {
		path = strings.ToLower(path)
	}
	return path
}

// Trust remembers an explicit confirmation for one file and exact content hash.
// Different files, external edits and new content never inherit permission.
func (s *Store) Trust(path, version string, trusted bool) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path = filepath.Clean(path)
	if !s.allowed(path) {
		return Document{}, errors.New("请先打开此脚本")
	}
	doc, err := read(path)
	if err != nil {
		return doc, err
	}
	if version == "" || doc.Version != version {
		return doc, errors.New("脚本已发生变化，请重新打开并确认信任")
	}
	next := make(map[string]string, len(s.trusted)+1)
	for key, value := range s.trusted {
		next[key] = value
	}
	if trusted {
		next[trustPath(path)] = version
	} else {
		delete(next, trustPath(path))
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err == nil {
		err = replace(filepath.Join(s.root, ".trusted.json"), data)
	}
	if err != nil {
		return doc, err
	}
	s.trusted = next
	doc.External, doc.Trusted = !s.managed(path), trusted
	return doc, nil
}
