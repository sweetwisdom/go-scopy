// Command assets renders the editable brand SVG using Viva's existing renderer.
package main

import (
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/headless"
	"github.com/vivaui/viva/tools/appbundle"
	"github.com/vivaui/viva/tools/iconcompile"
	"github.com/vivaui/viva/tools/winpkg"
	"github.com/vivaui/viva/tools/winres"
	"github.com/vivaui/viva/ui"
)

const assetDir = "internal/brand/assets"

func main() {
	stamp := flag.String("stamp-windows", "", "add the application icon to a freshly built Windows executable")
	flag.Parse()
	if err := run(*stamp); err != nil {
		log.Fatal(err)
	}
}

func run(stamp string) error {
	if stamp != "" {
		executable, err := os.ReadFile(stamp)
		if err != nil {
			return err
		}
		ico, _, err := winpkg.LoadIcon(filepath.Join(assetDir, "icon.ico"))
		if err != nil {
			return err
		}
		patched, err := winres.Patch(executable, ico, winres.Info{Name: "Go-Scopy", Description: "Go-Scopy Android Workspace", Version: "0.1.4", File: "Go-Scopy.exe"})
		if err != nil {
			return err
		}
		return os.WriteFile(stamp, patched, 0755)
	}
	source, err := os.ReadFile(filepath.Join(assetDir, "icon.svg"))
	if err != nil {
		return err
	}
	icon, err := iconcompile.Compile("Go-Scopy", source)
	if err != nil {
		return err
	}
	painted := icon
	session, err := headless.Open(app.Config{Title: "Go-Scopy icon", Width: 1024, Height: 1024, WindowMode: app.WindowFrameless, ClearColor: app.Color{Alpha: 1},
		Paint: func(c *ui.Canvas, s ui.Size) {
			c.DrawIcon(painted, ui.Rect{Width: s.Width, Height: s.Height}, ui.RGB(255, 255, 255))
		},
	}, nil, headless.Options{Scale: 1})
	if err != nil {
		return err
	}
	// Viva screenshots are opaque. A white silhouette on black recovers exact
	// antialiased coverage while the first render supplies premultiplied colors.
	defer session.Close()
	master, err := session.Image()
	if err != nil {
		return err
	}
	painted.Layers = append([]ui.IconLayer(nil), icon.Layers...)
	for i := range painted.Layers {
		painted.Layers[i].Fill = ui.RGB(255, 255, 255)
		painted.Layers[i].Gradient = nil
	}
	if err = session.Driver().Resize(1024, 1024); err != nil {
		return err
	}
	mask, err := session.Image()
	if err != nil {
		return err
	}
	for i := 3; i < len(master.Pix); i += 4 {
		master.Pix[i] = mask.Pix[i-3]
	}
	for _, size := range []int{16, 24, 32, 48, 64, 128, 256, 512, 1024} {
		name := "icon.png"
		if size != 1024 {
			name = fmt.Sprintf("icon-%d.png", size)
		}
		file, err := os.Create(filepath.Join(assetDir, name))
		if err != nil {
			return err
		}
		err = png.Encode(file, appbundle.Downscale(master, size))
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	ico, _, err := winpkg.EncodeICO(master)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(assetDir, "icon.ico"), ico, 0644)
}
