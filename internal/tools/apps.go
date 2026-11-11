package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/shogo82148/androidbinary"
	"github.com/shogo82148/androidbinary/apk"
)

var (
	packageLine = regexp.MustCompile(`^package:(.+)=([^=]+)$`)
	pkgBlock    = regexp.MustCompile(`^  Package \[([^\]]+)\]`)
	versionName = regexp.MustCompile(`^\s*versionName=(.*)$`)
	versionCode = regexp.MustCompile(`^\s*versionCode=(\d+)`)
)

// ListApps reads the installed packages: paths and system/user split from pm,
// versions from one dumpsys pass, sizes from batched du.
func (t Tools) ListApps(ctx context.Context, serial string) ([]AppInfo, error) {
	if apps, err := t.listAppsPanda(ctx, serial); err == nil && len(apps) > 0 {
		return apps, nil
	}
	// Keep initial listing responsive. Expensive package dumps and a du call
	// per APK directory are deliberately deferred from the list operation.
	all, err := t.packagePaths(ctx, serial, "pm list packages -f")
	if err != nil {
		return nil, err
	}
	third := map[string]bool{}
	if out, err := t.Device(ctx, serial, "shell", "pm list packages -3"); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if pkg, ok := strings.CutPrefix(strings.TrimSpace(line), "package:"); ok {
				third[pkg] = true
			}
		}
	}
	apps := make([]AppInfo, 0, len(all))
	for pkg, apkPath := range all {
		apps = append(apps, AppInfo{Package: pkg, APKPath: apkPath, System: !third[pkg]})
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].Package < apps[j].Package })
	return apps, nil
}

func (t Tools) packagePaths(ctx context.Context, serial, command string) (map[string]string, error) {
	out, err := t.Device(ctx, serial, "shell", command)
	if err != nil {
		return nil, err
	}
	paths := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		m := packageLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m != nil {
			paths[m[2]] = m[1]
		}
	}
	return paths, nil
}

type versionPair struct{ name, code string }

func parsePackageVersions(text string) map[string]versionPair {
	versions := map[string]versionPair{}
	current := ""
	for _, line := range strings.Split(text, "\n") {
		if m := pkgBlock.FindStringSubmatch(line); m != nil {
			current = m[1]
			continue
		}
		if current == "" {
			continue
		}
		if m := versionName.FindStringSubmatch(line); m != nil {
			v := versions[current]
			v.name = strings.TrimSpace(m[1])
			versions[current] = v
		} else if m := versionCode.FindStringSubmatch(line); m != nil {
			v := versions[current]
			v.code = m[1]
			versions[current] = v
		}
	}
	return versions
}

// AppMeta resolves one app's human label and icon from its base.apk. The APK's
// manifest and resource table are extracted on the device with unzip, so the
// full package is never pulled; if unzip is missing the whole APK is pulled
// once and parsed locally instead.
func (t Tools) AppMeta(ctx context.Context, serial, apkPath string) (string, []byte, error) {
	manifest, arsc, err := t.zipPair(ctx, serial, apkPath)
	if err != nil {
		if !errors.Is(err, errNoUnzip) {
			return "", nil, err
		}
		label, icon, err := t.appMetaByPull(ctx, serial, apkPath)
		return label, icon, err
	}
	table, err := androidbinary.NewTableFile(bytes.NewReader(arsc))
	if err != nil {
		return "", nil, fmt.Errorf("解析资源表失败：%w", err)
	}
	xmlFile, err := androidbinary.NewXMLFile(bytes.NewReader(manifest))
	if err != nil {
		return "", nil, fmt.Errorf("解析应用清单失败：%w", err)
	}
	var man apk.Manifest
	if err = xmlFile.Decode(&man, table, nil); err != nil {
		return "", nil, fmt.Errorf("解析应用清单失败：%w", err)
	}
	// A Chinese-capable config picks localized labels and PNG density variants
	// over adaptive-icon XMLs where both exist.
	config := &androidbinary.ResTableConfig{Density: 480}
	config.Language = [2]uint8{'z', 'h'}
	label := resolveString(man.App.Label, table, config)
	if label == "" {
		label = man.Package.MustString()
	}
	iconPath := resolveString(man.App.Icon, table, config)
	icon := t.iconFromZip(ctx, serial, apkPath, iconPath)
	return label, icon, nil
}

var errNoUnzip = errors.New("设备上没有 unzip")

func resolveString(ref androidbinary.String, table *androidbinary.TableFile, config *androidbinary.ResTableConfig) string {
	value, err := ref.WithResTableConfig(config).String()
	if err != nil || androidbinary.IsResID(value) {
		return ""
	}
	return value
}

// zipPair extracts AndroidManifest.xml and resources.arsc from an APK on the
// device. Two exec-out calls: unzip streams only the requested entries.
func (t Tools) zipPair(ctx context.Context, serial, apkPath string) ([]byte, []byte, error) {
	manifest, err := t.zipEntry(ctx, serial, apkPath, "AndroidManifest.xml")
	if err != nil {
		return nil, nil, err
	}
	arsc, err := t.zipEntry(ctx, serial, apkPath, "resources.arsc")
	if err != nil {
		return nil, nil, err
	}
	return manifest, arsc, nil
}

func (t Tools) zipEntry(ctx context.Context, serial, apkPath, name string) ([]byte, error) {
	out, err := t.Device(ctx, serial, "exec-out", "unzip -p "+shellQuote(apkPath)+" "+shellQuote(name))
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "inaccessible") {
			return nil, errNoUnzip
		}
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("APK 内没有 %s", name)
	}
	return out, nil
}

// iconFromZip downloads one icon resource and normalizes it to PNG bytes.
func (t Tools) iconFromZip(ctx context.Context, serial, apkPath, iconPath string) []byte {
	if iconPath == "" || !strings.HasPrefix(iconPath, "res/") || strings.HasSuffix(iconPath, ".xml") {
		return nil // adaptive-icon XML: no raster form without rendering it
	}
	data, err := t.zipEntry(ctx, serial, apkPath, iconPath)
	if err != nil {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

// appMetaByPull is the fallback for devices without unzip: pull the APK, parse
// it locally, and delete the copy.
func (t Tools) appMetaByPull(ctx context.Context, serial, apkPath string) (string, []byte, error) {
	temp, err := tempFilePath(path.Base(apkPath))
	if err != nil {
		return "", nil, err
	}
	defer removeQuiet(temp)
	if _, err := t.Pull(ctx, serial, apkPath, temp); err != nil {
		return "", nil, err
	}
	handle, err := apk.OpenFile(temp)
	if err != nil {
		return "", nil, err
	}
	defer handle.Close()
	config := &androidbinary.ResTableConfig{Density: 480}
	config.Language = [2]uint8{'z', 'h'}
	label, err := handle.Label(config)
	if err != nil {
		label = handle.PackageName()
	}
	var icon []byte
	if img, err := handle.Icon(config); err == nil {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err == nil {
			icon = buf.Bytes()
		}
	}
	return label, icon, nil
}

// AppPaths lists every APK of a package: the base plus any split APKs, which
// export and backup need to pull together.
func (t Tools) AppPaths(ctx context.Context, serial, pkg string) ([]string, error) {
	out, err := t.Device(ctx, serial, "shell", "pm path "+shellQuote(pkg))
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "package:"); ok {
			paths = append(paths, rest)
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("没有找到 %s 的安装包", pkg)
	}
	return paths, nil
}

// Uninstall removes a user app outright; system apps can only have their
// updates removed for the current user.
func (t Tools) Uninstall(ctx context.Context, serial, pkg string, system bool) (string, error) {
	command := "pm uninstall"
	if system {
		command += " --user 0"
	}
	out, err := t.Device(ctx, serial, "shell", command+" "+shellQuote(pkg))
	return strings.TrimSpace(string(out)), err
}

func chunkStrings(values []string, size int) [][]string {
	var chunks [][]string
	for len(values) > 0 {
		n := min(size, len(values))
		chunks = append(chunks, values[:n])
		values = values[n:]
	}
	return chunks
}

func shellQuoteList(values []string) []string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = shellQuote(v)
	}
	return quoted
}
