package tools

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"runtime/debug"
	"time"
)

// RunAppMetaWorker handles the internal desktop subprocess before GUI startup.
// The legacy resource parser is confined to this short-lived worker.
func RunAppMetaWorker(args []string) (handled bool) {
	if len(args) == 0 || args[0] != "--app-meta-worker" {
		return false
	}
	handled = true
	if len(args) != 5 {
		return true
	}
	debug.SetMemoryLimit(128 << 20)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	defer func() { _ = recover() }()
	label, icon, err := (Tools{ADB: args[1]}).AppMeta(ctx, args[2], args[3])
	if err == nil {
		_ = json.NewEncoder(os.Stdout).Encode(AppInfo{Package: args[4], Label: label, IconPNG: icon})
	}
	return true
}

func (t Tools) IsolatedAppMeta(ctx context.Context, serial, apkPath, pkg string) (AppInfo, bool) {
	exe, err := os.Executable()
	if err != nil {
		return AppInfo{}, false
	}
	command := Command(ctx, exe, "--app-meta-worker", t.ADB, serial, apkPath, pkg)
	pipe, err := command.StdoutPipe()
	if err != nil {
		return AppInfo{}, false
	}
	if command.Start() != nil {
		return AppInfo{}, false
	}
	data, err := io.ReadAll(io.LimitReader(pipe, 6<<20))
	// A bound on IPC output also bounds retained icon bytes in the desktop.
	if len(data) >= 6<<20 {
		_ = command.Process.Kill()
	}
	waitErr := command.Wait()
	var app AppInfo
	if err != nil || waitErr != nil || json.Unmarshal(data, &app) != nil || app.Package != pkg {
		return AppInfo{}, false
	}
	return app, len(app.IconPNG) > 0
}
