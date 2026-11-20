package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vivaui/viva/tools/winpkg"
)

// Reuse Viva's user-scope NSIS installer (architecture checks, shortcuts,
// registry, close-app protection). Extend only its payload for bundled tools.
func windowsInstallerScript(root, version, output string) ([]byte, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return nil, err
	}
	script, err := (winpkg.Script{Name: "Go-Scopy", Version: version, Publisher: "Go-Scopy", ID: "dev.goscopy.desktop", Scope: winpkg.ScopeUser, Executable: "Go-Scopy.exe", Program: filepath.Join(root, "Go-Scopy.exe"), Icon: filepath.Join(root, "Go-Scopy.ico"), Output: output, Arch: "amd64"}).Generate()
	if err != nil {
		return nil, err
	}
	escape := func(value string) (string, error) {
		if strings.ContainsAny(value, "\r\n\x00") {
			return "", fmt.Errorf("invalid NSIS path")
		}
		return strings.ReplaceAll(strings.ReplaceAll(value, "$", "$$"), `"`, `$\"`), nil
	}
	var files []string
	dirs := map[string]bool{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("installer payload must be regular files: %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !filepath.IsLocal(rel) {
			return fmt.Errorf("payload escaped package root")
		}
		if rel == "Go-Scopy.exe" || rel == "Go-Scopy.ico" {
			return nil
		}
		files = append(files, rel)
		for dir := filepath.Dir(rel); dir != "."; dir = filepath.Dir(dir) {
			dirs[dir] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var install, uninstall strings.Builder
	for _, rel := range files {
		windowsRel := strings.ReplaceAll(filepath.ToSlash(rel), "/", `\`)
		dir := filepath.Dir(rel)
		target := `$INSTDIR`
		if dir != "." {
			escaped, err := escape(strings.ReplaceAll(filepath.ToSlash(dir), "/", `\`))
			if err != nil {
				return nil, err
			}
			target += `\` + escaped
		}
		source, err := escape(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		name, err := escape(filepath.Base(rel))
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&install, "  SetOutPath \"%s\"\n  File \"/oname=%s\" \"%s\"\n", target, name, source)
		escaped, err := escape(windowsRel)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&uninstall, "  Delete \"$INSTDIR\\%s\"\n", escaped)
	}
	// Never use recursive removal: unknown/user-created files must survive uninstall.
	var directories []string
	for dir := range dirs {
		directories = append(directories, dir)
	}
	sort.Slice(directories, func(i, j int) bool {
		if len(directories[i]) == len(directories[j]) {
			return directories[i] > directories[j]
		}
		return len(directories[i]) > len(directories[j])
	})
	for _, dir := range directories {
		escaped, err := escape(strings.ReplaceAll(filepath.ToSlash(dir), "/", `\`))
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&uninstall, "  RMDir \"$INSTDIR\\%s\"\n", escaped)
	}
	install.WriteString("  SetOutPath \"$INSTDIR\"\n")
	text := string(script)
	// Add the standard key omitted by Viva's base template (avoids NSIS 9100).
	for _, language := range []string{"ENGLISH", "SIMPCHINESE"} {
		anchor := fmt.Sprintf(`VIAddVersionKey /LANG=${LANG_%s} "ProductName" "${APP_NAME}"`, language)
		text = strings.Replace(text, anchor, anchor+"\n"+fmt.Sprintf(`VIAddVersionKey /LANG=${LANG_%s} "LegalCopyright" "Go-Scopy contributors"`, language), 1)
	}

	const installAnchor = `  WriteUninstaller "$INSTDIR\Uninstall.exe"`
	const uninstallAnchor = `  Delete "$INSTDIR\Uninstall.exe"`
	if strings.Count(text, installAnchor) != 1 || strings.Count(text, uninstallAnchor) != 1 {
		return nil, fmt.Errorf("Viva NSIS template changed; refusing to generate incomplete installer")
	}
	text = strings.Replace(text, installAnchor, install.String()+installAnchor, 1)
	text = strings.Replace(text, uninstallAnchor, uninstall.String()+uninstallAnchor, 1)
	return append([]byte{0xef, 0xbb, 0xbf}, []byte(text)...), nil
}
func buildWindowsInstaller(root, version, out, compiler string) error {
	output := filepath.Join(out, "Go-Scopy-"+version+"-windows-amd64-setup.exe")
	script, err := windowsInstallerScript(root, version, output)
	if err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(root), "installer.nsi")
	if err = os.WriteFile(path, script, 0600); err != nil {
		return err
	}
	if err = run(compiler, "/V2", path); err != nil {
		return fmt.Errorf("NSIS build: %w", err)
	}
	sum, err := fileHash(output)
	if err != nil {
		return err
	}
	return os.WriteFile(output+".sha256", []byte(sum+"  "+filepath.Base(output)+"\n"), 0644)
}
