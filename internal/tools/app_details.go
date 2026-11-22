package tools

import (
	"context"
	"path"
	"strconv"
	"strings"
	"time"
)

// AppPackageDetails reads sortable installation facts without rendering icons.
// Size matches the Agent's base-APK byte count, rather than including app data.
func (t Tools) AppPackageDetails(ctx context.Context, serial string, apps []AppInfo) map[string]AppInfo {
	details := make(map[string]AppInfo, len(apps))
	for _, app := range apps {
		app.IconPNG = nil
		details[app.Package] = app
	}
	if out, err := t.Device(ctx, serial, "shell", "dumpsys package"); err == nil {
		parseAppDetails(string(out), details)
	}
	byPath := map[string][]string{}
	var paths []string
	for _, app := range apps {
		if !strings.HasPrefix(app.APKPath, "/") || validPath(app.APKPath) != nil || path.Ext(app.APKPath) != ".apk" {
			continue
		}
		if _, exists := byPath[app.APKPath]; !exists {
			paths = append(paths, app.APKPath)
		}
		byPath[app.APKPath] = append(byPath[app.APKPath], app.Package)
	}
	for _, chunk := range chunkStrings(paths, 64) {
		out, _ := t.Device(ctx, serial, "shell", "stat -c '%s %n' -- "+strings.Join(shellQuoteList(chunk), " "))
		for _, line := range strings.Split(string(out), "\n") {
			sizeText, file, ok := strings.Cut(strings.TrimSpace(line), " ")
			size, err := strconv.ParseInt(sizeText, 10, 64)
			if !ok || err != nil || size < 0 {
				continue
			}
			for _, pkg := range byPath[file] {
				app := details[pkg]
				app.SizeBytes = size
				details[pkg] = app
			}
		}
	}
	return details
}

func parseAppDetails(text string, apps map[string]AppInfo) {
	current := ""
	for _, line := range strings.Split(text, "\n") {
		if match := pkgBlock.FindStringSubmatch(line); match != nil {
			current = match[1]
			continue
		}
		app, exists := apps[current]
		if !exists {
			continue
		}
		if match := versionName.FindStringSubmatch(line); match != nil {
			app.VersionName = strings.TrimSpace(match[1])
		}
		if match := versionCode.FindStringSubmatch(line); match != nil {
			app.VersionCode = match[1]
		}
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "firstInstallTime="); ok {
			if timestamp, err := time.ParseInLocation(time.DateTime, strings.TrimSpace(value), time.Local); err == nil {
				app.InstalledAt = timestamp
			}
		}
		apps[current] = app
	}
}
