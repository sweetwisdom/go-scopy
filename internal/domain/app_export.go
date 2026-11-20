package domain

import "strings"

// AppExportName sanitizes the display name and version, never accepting a device-supplied path.
func AppExportName(label, version string) string {
	clean := func(s string) string {
		return strings.Trim(strings.Map(func(r rune) rune {
			if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
				return '_'
			}
			return r
		}, s), " .")
	}
	name := clean(label)
	if version = clean(version); version != "" {
		name += "-" + version
	}
	runes := []rune(name)
	if len(runes) > 180 {
		name = string(runes[:180])
	}
	return name + ".apk"
}

// AppExportFilename prefers the visible app label; the package is only a missing-label fallback.
func AppExportFilename(info AppInfo, meta AppMeta) string {
	label := strings.TrimSpace(meta.Label)
	if label == "" {
		label = strings.TrimSpace(info.Label)
	}
	if label == "" {
		label = info.Package
	}
	version := info.VersionName
	if version == "" {
		version = info.VersionCode
	}
	return AppExportName(label, version)
}
