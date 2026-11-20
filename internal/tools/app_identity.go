package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
)

// AppFingerprint includes every APK (including resource splits), resource
// configuration and Android API level. A package/version alone is NOT identity:
// manufacturers and alternate builds may reuse both with different resources.
func (t Tools) AppFingerprint(ctx context.Context, serial, pkg string) (string, string, error) {
	paths, err := t.AppPaths(ctx, serial, pkg)
	if err != nil {
		return "", "", err
	}
	sort.Strings(paths)
	command := "getprop ro.build.version.sdk; getprop persist.sys.locale; sha256sum " + strings.Join(shellQuoteList(paths), " ")
	out, err := t.Device(ctx, serial, "shell", command)
	if err != nil {
		return "", "", err
	}
	lines := strings.Split(strings.TrimRight(string(out), "\r\n"), "\n")
	if len(lines) != len(paths)+2 {
		return "", "", errors.New("应用校验信息不完整")
	}
	identity := []string{"app-resources-v2:zh:480", pkg, strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])}
	base := paths[0]
	for _, line := range lines[2:] {
		fields := strings.Fields(line)
		if len(fields) < 2 || len(fields[0]) != 64 {
			return "", "", errors.New("设备不支持应用 SHA256 校验")
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			return "", "", err
		}
		identity = append(identity, strings.ToLower(fields[0]))
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "/base.apk") {
			base = path
			break
		}
	}
	// Absolute installation paths differ between devices; only verified content
	// hashes participate in the global key, not transport or install directory.
	sum := sha256.Sum256([]byte(strings.Join(identity, "\x00")))
	return hex.EncodeToString(sum[:]), base, nil
}
