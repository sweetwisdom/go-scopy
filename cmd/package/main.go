// Command package builds native release archives. Archive and architecture
// helpers are adapted from GoIM (MIT; see licenses/GoIM-MIT.txt).
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/go-scopy/go-scopy/internal/buildinfo"
	"github.com/vivaui/viva/tools/appbundle"
	"github.com/vivaui/viva/tools/winpkg"
	"github.com/vivaui/viva/tools/winres"
)

func main() {
	if err := build(); err != nil {
		log.Fatal(err)
	}
}

func run(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	return c.Run()
}

func build() error {
	version := flag.String("version", buildinfo.Version, "release version, without v")
	commit := flag.String("commit", "working-tree", "source commit")
	out := flag.String("out", "dist", "output directory")
	flag.Parse()
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(*version) {
		return fmt.Errorf("invalid release version")
	}
	platform := map[string]string{"windows/amd64": "win", "linux/amd64": "linux-x64", "linux/arm64": "linux-arm64", "darwin/amd64": "mac-x64", "darwin/arm64": "mac-arm64"}[runtime.GOOS+"/"+runtime.GOARCH]
	if platform == "" {
		return fmt.Errorf("unsupported native target %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if err := verifyResources(); err != nil {
		return err
	}
	name := "Go-Scopy-" + *version + "-" + runtime.GOOS + "-" + runtime.GOARCH
	// A fresh staging directory prevents old files entering a new release.
	if err := os.MkdirAll(*out, 0755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(*out, ".package-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	root := filepath.Join(staging, name)
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	exe := filepath.Join(root, "Go-Scopy")
	ld := "-s -w -X github.com/go-scopy/go-scopy/internal/buildinfo.Version=" + *version
	if runtime.GOOS == "windows" {
		exe += ".exe"
		ld += " -H=windowsgui"
	}
	if err := run("go", "build", "-trimpath", "-ldflags", ld, "-o", exe, "./cmd/go-scopy"); err != nil {
		return err
	}
	if err := verifyArchitecture(exe); err != nil {
		return err
	}
	toolsRoot := filepath.Join(root, "tools", "scrcpy")
	if runtime.GOOS == "darwin" {
		bundle := filepath.Join(root, "Go-Scopy.app")
		_, err := appbundle.Assemble(appbundle.Options{Dest: bundle, Executable: exe, Icon: "internal/brand/assets/icon.png", Info: appbundle.Info{Name: "Go-Scopy", ID: "dev.goscopy.desktop", Version: *version, Executable: "Go-Scopy", MinSystem: "13.0", Category: "public.app-category.utilities"}})
		if err != nil {
			return err
		}
		if err := os.Remove(exe); err != nil {
			return err
		}
		toolsRoot = filepath.Join(bundle, "Contents", "Resources", "tools", "scrcpy")
	} else if runtime.GOOS == "windows" {
		ico, _, err := winpkg.LoadIcon("internal/brand/assets/icon.ico")
		if err != nil {
			return err
		}
		data, err := os.ReadFile(exe)
		if err != nil {
			return err
		}
		data, err = winres.Patch(data, ico, winres.Info{Name: "Go-Scopy", Description: "Go-Scopy Android Workspace", Version: *version, File: "Go-Scopy.exe"})
		if err != nil {
			return err
		}
		if err := os.WriteFile(exe, data, 0755); err != nil {
			return err
		}
		if err := copyFile("internal/brand/assets/icon.ico", filepath.Join(root, "Go-Scopy.ico")); err != nil {
			return err
		}
	}
	if err := copyTree(filepath.Join("resources", "extra", platform, "scrcpy"), toolsRoot); err != nil {
		return err
	}
	if err := copyFile("resources/extra/common/scrcpy/scrcpy-server", filepath.Join(toolsRoot, "scrcpy-server")); err != nil {
		return err
	}
	if err := copyFile("resources/manifest.json", filepath.Join(toolsRoot, "source.json")); err != nil {
		return err
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	for _, tool := range []string{"adb", "scrcpy", "fastboot"} {
		path := filepath.Join(toolsRoot, tool+suffix)
		if err := os.Chmod(path, 0755); err != nil {
			return err
		}
		if err := verifyToolArchitecture(path); err != nil {
			return err
		}
	}
	if runtime.GOOS == "darwin" {
		for _, tool := range []string{"adb", "scrcpy", "fastboot"} {
			if err := run("codesign", "--force", "--sign", "-", filepath.Join(toolsRoot, tool)); err != nil {
				return err
			}
		}
	}
	for _, tool := range []string{"adb", "scrcpy"} {
		path, err := filepath.Abs(filepath.Join(toolsRoot, tool+suffix))
		if err != nil {
			return err
		}
		arg := "--version"
		if tool == "adb" {
			arg = "version"
		}
		if err := run(path, arg); err != nil {
			return fmt.Errorf("bundled %s cannot run: %w", tool, err)
		}
	}
	if runtime.GOOS == "darwin" {
		bundle := filepath.Join(root, "Go-Scopy.app")
		if err := run("codesign", "--force", "--sign", "-", bundle); err != nil {
			return err
		}
		if err := run("codesign", "--verify", "--deep", "--strict", bundle); err != nil {
			return err
		}
	}
	for _, doc := range []string{"README.md", "LICENSE", "THIRD_PARTY_NOTICES.md"} {
		if err := copyFile(doc, filepath.Join(root, doc)); err != nil {
			return err
		}
	}
	for _, dir := range []string{"licenses", "docs"} {
		if err := copyTree(dir, filepath.Join(root, dir)); err != nil {
			return err
		}
	}
	info := map[string]string{"version": *version, "commit": *commit, "os": runtime.GOOS, "arch": runtime.GOARCH, "go": runtime.Version(), "viva": "v0.2.3", "scrcpy": "4.1", "toolSource": "escrcpy 3a73e0d4de58a4674dddd61fc43956b4882c8bec", "validation": "native build and bundled tool versions; physical device acceptance is separate"}
	b, _ := json.MarshalIndent(info, "", "  ")
	if err := os.WriteFile(filepath.Join(root, "build-info.json"), b, 0644); err != nil {
		return err
	}
	if err := writeChecksums(root); err != nil {
		return err
	}
	ext := ".zip"
	if runtime.GOOS == "linux" {
		ext = ".tar.gz"
	}
	archive := filepath.Join(*out, name+ext)
	if err := pack(root, archive, runtime.GOOS == "linux"); err != nil {
		return err
	}
	sum, err := fileHash(archive)
	if err != nil {
		return err
	}
	return os.WriteFile(archive+".sha256", []byte(sum+"  "+filepath.Base(archive)+"\n"), 0644)
}

func verifyResources() error {
	var manifest struct {
		Files []struct{ Path, SHA256 string }
	}
	b, err := os.ReadFile("resources/manifest.json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		return err
	}
	if len(manifest.Files) == 0 {
		return fmt.Errorf("empty resource manifest")
	}
	for _, f := range manifest.Files {
		if !filepath.IsLocal(filepath.FromSlash(f.Path)) {
			return fmt.Errorf("invalid resource path")
		}
		sum, err := fileHash(filepath.Join("resources", "extra", filepath.FromSlash(f.Path)))
		if err != nil {
			return err
		}
		if sum != f.SHA256 {
			return fmt.Errorf("resource checksum mismatch: %s", f.Path)
		}
	}
	return nil
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeChecksums(root string) error {
	var lines []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		sum, err := fileHash(path)
		if err != nil {
			return err
		}
		lines = append(lines, sum+"  "+filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "SHA256SUMS.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

func copyTree(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		return copyFile(path, filepath.Join(dest, rel))
	})
}

// Google's Windows adb/fastboot are x86 executables supported by Windows x64.
func verifyToolArchitecture(path string) error {
	if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
		f, err := pe.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if f.Machine == pe.IMAGE_FILE_MACHINE_I386 {
			return nil
		}
	}
	return verifyArchitecture(path)
}

func verifyArchitecture(path string) error {
	valid := false
	switch runtime.GOOS {
	case "windows":
		f, err := pe.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		valid = runtime.GOARCH == "amd64" && f.Machine == pe.IMAGE_FILE_MACHINE_AMD64 || runtime.GOARCH == "arm64" && f.Machine == pe.IMAGE_FILE_MACHINE_ARM64
	case "linux":
		f, err := elf.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		valid = runtime.GOARCH == "amd64" && f.Machine == elf.EM_X86_64 || runtime.GOARCH == "arm64" && f.Machine == elf.EM_AARCH64
	case "darwin":
		matches := func(cpu macho.Cpu) bool {
			return runtime.GOARCH == "amd64" && cpu == macho.CpuAmd64 || runtime.GOARCH == "arm64" && cpu == macho.CpuArm64
		}
		if fat, err := macho.OpenFat(path); err == nil {
			defer fat.Close()
			for _, arch := range fat.Arches {
				valid = valid || matches(arch.Cpu)
			}
			break
		}
		f, err := macho.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		valid = matches(f.Cpu)
	}
	if !valid {
		return fmt.Errorf("binary %s does not match native target %s/%s", path, runtime.GOOS, runtime.GOARCH)
	}
	return nil
}
func copyFile(src, dest string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	return os.WriteFile(dest, b, 0644)
}
func pack(root, dest string, tgz bool) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	var z *zip.Writer
	var tw *tar.Writer
	var gz *gzip.Writer
	if tgz {
		gz = gzip.NewWriter(f)
		tw = tar.NewWriter(gz)
	} else {
		z = zip.NewWriter(f)
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(filepath.Dir(root), path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		var w io.Writer
		if tgz {
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = rel
			hdr.ModTime = time.Unix(0, 0)
			if err = tw.WriteHeader(hdr); err != nil {
				return err
			}
			w = tw
		} else {
			hdr, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			hdr.Name = rel
			hdr.Method = zip.Deflate
			hdr.Modified = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
			w, err = z.CreateHeader(hdr)
			if err != nil {
				return err
			}
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, input)
		input.Close()
		return err
	})
	if err != nil {
		return err
	}
	if tgz {
		if err = tw.Close(); err != nil {
			return err
		}
		return gz.Close()
	}
	return z.Close()
}
