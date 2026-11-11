package tools

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var lsLine = regexp.MustCompile(`^(\S{10,})\s+\d+\s+\S+\s+\S+\s+(\d+)\s+(\d{4}-\d{2}-\d{2})\s+(\d{2}:\d{2})\s+(.+)$`)

// ListFiles lists one directory on the device. toybox `ls` prints ISO dates,
// and `-p` marks directories with a trailing slash so names with spaces stay
// inside the last field.
func (t Tools) ListFiles(ctx context.Context, serial, dir string) ([]FileEntry, error) {
	if strings.TrimSpace(dir) == "" {
		dir = "/"
	}
	// Add a trailing slash so toybox follows directory symlinks such as /sdcard
	// instead of returning the symlink itself as a single file row.
	if dir != "/" {
		dir = strings.TrimRight(dir, "/") + "/"
	}
	out, err := t.Device(ctx, serial, "shell", "ls -la -p -- "+shellQuote(dir))
	if err != nil {
		return nil, err
	}
	var entries []FileEntry
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "total ") {
			continue
		}
		m := lsLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[5]
		isDir := m[1][0] == 'd' || strings.HasSuffix(name, "/")
		name = strings.TrimSuffix(name, "/")
		// A symlink row reads "name -> target"; keep the link itself.
		if i := strings.Index(name, " -> "); i >= 0 {
			name = name[:i]
			isDir = false
		}
		size, _ := strconv.ParseInt(m[2], 10, 64)
		entries = append(entries, FileEntry{Name: name, IsDir: isDir, Size: size, ModTime: m[3] + " " + m[4]})
	}
	return entries, nil
}

// MakeDir creates a directory, including parents (mkdir -p).
func (t Tools) MakeDir(ctx context.Context, serial, path string) error {
	if err := validPath(path); err != nil {
		return err
	}
	_, err := t.Device(ctx, serial, "shell", "mkdir -p "+shellQuote(path))
	return err
}

// Rename moves or renames one path on the device.
func (t Tools) RenamePath(ctx context.Context, serial, from, to string) error {
	if err := validPath(from); err != nil {
		return err
	}
	if err := validPath(to); err != nil {
		return err
	}
	_, err := t.Device(ctx, serial, "shell", "mv "+shellQuote(from)+" "+shellQuote(to))
	return err
}

// RemovePath deletes a file, or a directory tree when recursive is set. The
// caller is responsible for confirming with the user first.
func (t Tools) RemovePath(ctx context.Context, serial, path string, recursive bool) error {
	if err := validPath(path); err != nil {
		return err
	}
	if path == "/" {
		return errors.New("不允许删除设备根目录")
	}
	command := "rm -f"
	if recursive {
		command = "rm -rf"
	}
	_, err := t.Device(ctx, serial, "shell", command+" "+shellQuote(path))
	return err
}

// Pull downloads one remote path into a local file or directory.
func (t Tools) Pull(ctx context.Context, serial, remote, local string) (string, error) {
	if err := validPath(remote); err != nil {
		return "", err
	}
	out, err := t.Device(ctx, serial, "pull", remote, local)
	return strings.TrimSpace(string(out)), err
}

// Push uploads a local file to a remote directory or file path.
func (t Tools) Push(ctx context.Context, serial, local, remote string) (string, error) {
	if err := validPath(remote); err != nil {
		return "", err
	}
	out, err := t.Device(ctx, serial, "push", local, remote)
	return strings.TrimSpace(string(out)), err
}

// validPath keeps shell metacharacters out of device paths; every command is
// quoted, this is the second layer.
func validPath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("设备路径不能为空")
	}
	if strings.ContainsAny(path, "\x00\n\r") {
		return errors.New("设备路径包含无效字符")
	}
	return nil
}

// shellQuote wraps one argument in single quotes for the device shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func errUnsupported(message string) error { return fmt.Errorf("%s", message) }
