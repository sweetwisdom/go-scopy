// Command go-scopy-script runs the same script engine without the desktop UI.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/go-scopy/go-scopy/internal/automation"
	"github.com/go-scopy/go-scopy/internal/config"
	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/tools"
)

func main() { os.Exit(run()) }
func run() int {
	serial := flag.String("serial", "", "required device serial")
	script := flag.String("script", "", "required .py file")
	python := flag.String("python", "", "Python override (default from PATH)")
	root := flag.String("tools-root", "", "application directory (default working directory)")
	dir := flag.String("data-dir", "", "configuration and run-history directory")
	timeout := flag.Duration("timeout", 10*time.Minute, "script execution timeout (1s–24h)")
	flag.Parse()
	if *serial == "" || *script == "" {
		flag.Usage()
		return 2
	}
	if *timeout < time.Second || *timeout > 24*time.Hour {
		fmt.Fprintln(os.Stderr, "timeout must be 1s–24h")
		return 2
	}
	if *root == "" {
		*root, _ = os.Getwd()
	}
	if *dir == "" {
		var err error
		*dir, err = config.Directory()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	settings, err := config.Load(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	adb, err := tools.Resolve(settings.ADBPath, "adb", *root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// Start the shared adb server outside the script process tree.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	_, err = (tools.Tools{ADB: adb}).Run(ctx, "start-server")
	cancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	changed := make(chan struct{}, 1)
	m := automation.New(*root, *dir, adb, func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	defer m.Close()
	id, err := m.Start(domain.ScriptRequest{Path: *script, Serial: *serial, Python: *python, TimeoutSeconds: int(*timeout / time.Second), Args: flag.Args()})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Fprintln(os.Stderr, "Run:", id)
	signals, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	sig := signals.Done()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	var log *os.File
	defer func() {
		if log != nil {
			log.Close()
		}
	}()
	for {
		snapshot := m.Snapshot()
		for _, r := range snapshot.Runs {
			if r.ID != id {
				continue
			}
			if log == nil {
				log, _ = os.Open(filepath.Clean(r.LogPath))
			}
			if log != nil {
				_, _ = io.Copy(os.Stdout, log)
			}
			if !r.Active() {
				if r.Error != "" {
					fmt.Fprintln(os.Stderr, r.Error)
				}
				if r.Status == "success" {
					return 0
				}
				if r.Status == "cancelled" {
					return 130
				}
				if r.Status == "timeout" {
					return 124
				}
				return 1
			}
		}
		select {
		case <-changed:
		case <-ticker.C:
		case <-sig:
			m.Stop(id)
			sig = nil
		}
	}
}
