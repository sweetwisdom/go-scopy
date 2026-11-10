// Package brand supplies the same application identity to native and painted UI.
package brand

import (
	_ "embed"
	"sync"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/app/iconpng"
	"github.com/vivaui/viva/ui"
)

//go:embed assets/icon-256.png
var pixels []byte

var nativeIcon = sync.OnceValue(func() app.WindowIcon {
	icon, err := iconpng.Decode(pixels)
	if err != nil {
		panic(err)
	}
	return icon
})

func WindowIcon() app.WindowIcon { return nativeIcon() }

var bitmap = sync.OnceValue(func() ui.Bitmap {
	i := nativeIcon()
	bgra := make([]byte, len(i.RGBA))
	for p := 0; p < len(bgra); p += 4 {
		a := uint32(i.RGBA[p+3])
		bgra[p] = byte((uint32(i.RGBA[p+2])*a + 127) / 255)
		bgra[p+1] = byte((uint32(i.RGBA[p+1])*a + 127) / 255)
		bgra[p+2] = byte((uint32(i.RGBA[p])*a + 127) / 255)
		bgra[p+3] = byte(a)
	}
	return ui.Bitmap{ID: 0x4753434F5059, Version: 1, Width: i.Size, Height: i.Size, Stride: i.Size * 4, Pix: bgra}
})

func Bitmap() ui.Bitmap { return bitmap() }
