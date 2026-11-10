package main

import (
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/go-scopy/go-scopy/internal/config"
	"github.com/go-scopy/go-scopy/internal/core"
	"github.com/go-scopy/go-scopy/internal/desktop"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"
)

func main() {
	dir := flag.String("data-dir", "", "configuration directory (defaults to the user config directory)")
	root := flag.String("tools-root", "", "directory containing tools/scrcpy (defaults to executable directory)")
	flag.Parse()
	if *dir == "" {
		value, err := config.Directory()
		if err != nil {
			log.Fatal(err)
		}
		*dir = value
	}
	if err := os.MkdirAll(*dir, 0700); err != nil {
		log.Fatal(err)
	}
	if file, err := os.OpenFile(filepath.Join(*dir, "startup.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600); err == nil {
		defer file.Close()
		log.SetOutput(io.MultiWriter(os.Stderr, file))
	}
	if *root == "" {
		exe, err := os.Executable()
		if err != nil {
			log.Fatal(err)
		}
		*root = filepath.Dir(exe)
	}
	backend, err := core.New(*dir, *root)
	if err != nil {
		log.Print(err)
		return
	}
	defer backend.Close()
	shell := desktop.New(backend)
	var canvas ui.Canvas
	if err = app.RunApplication(func(application *app.Application) error {
		shell.SetApplication(application)
		if _, err := application.OpenWindow(shell.Config(), &canvas); err != nil {
			return err
		}
		// Collect results that completed before the window bound its invalidator.
		shell.Wake()
		return nil
	}); err != nil {
		log.Print(err)
	}
}
