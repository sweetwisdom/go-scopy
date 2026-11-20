package desktop

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/vivaui/viva/headless"
)

func bitmapTestPNG(tb testing.TB, c color.NRGBA, size int) []byte {
	tb.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		tb.Fatal(err)
	}
	return out.Bytes()
}

func TestAppBitmapCacheReusesPixelsAndInvalidatesPerDevice(t *testing.T) {
	var cache appBitmapCache
	data := bitmapTestPNG(t, color.NRGBA{R: 200, A: 255}, 192)
	first, ok := cache.bitmap("phone-one\x00com.example", data)
	if !ok || first.Width != 96 || first.Height != 96 {
		t.Fatal("large icon not bounded", first.Width, first.Height)
	}
	same, _ := cache.bitmap("phone-one\x00com.example", data)
	cloned, _ := cache.bitmap("phone-one\x00com.example", bytes.Clone(data))
	if &first.Pix[0] != &same.Pix[0] || &first.Pix[0] != &cloned.Pix[0] {
		t.Fatal("unchanged icon decoded again")
	}
	other, _ := cache.bitmap("phone-two\x00com.example", data)
	if first.ID == other.ID {
		t.Fatal("two devices shared GPU bitmap identity")
	}
	changed, _ := cache.bitmap("phone-one\x00com.example", bitmapTestPNG(t, color.NRGBA{B: 200, A: 255}, 192))
	if changed.ID != first.ID || changed.Version == first.Version || bytes.Equal(first.Pix, changed.Pix) {
		t.Fatal("updated icon did not invalidate")
	}
	if _, ok := cache.bitmap("phone-one\x00com.example", []byte("bad PNG")); ok {
		t.Fatal("bad icon accepted")
	}
	recovered, ok := cache.bitmap("phone-one\x00com.example", data)
	if !ok || recovered.Version != first.Version {
		t.Fatal("valid icon did not recover after corrupt content")
	}
}

func TestAppBitmapCacheEvictsOldEntries(t *testing.T) {
	var cache appBitmapCache
	data := bitmapTestPNG(t, color.NRGBA{G: 255, A: 255}, 1)
	for i := 0; i < appBitmapCacheEntries+20; i++ {
		cache.bitmap(fmt.Sprint(i), data)
	}
	if len(cache.entries) != appBitmapCacheEntries || cache.recent.Len() != appBitmapCacheEntries || cache.bytes > appBitmapCacheLimit {
		t.Fatal("cache is not bounded")
	}
	if cache.entries["0"] != nil || cache.entries[fmt.Sprint(appBitmapCacheEntries+19)] == nil {
		t.Fatal("LRU evicted wrong icon")
	}
}

func TestPackageGrid256ReusesDecodedIcons(t *testing.T) {
	data := bitmapTestPNG(t, color.NRGBA{R: 140, G: 80, B: 200, A: 255}, 192)
	f := sample()
	apps := make([]domain.AppInfo, 256)
	for i := range apps {
		apps[i] = domain.AppInfo{Package: fmt.Sprintf("com.example.app%03d", i), Label: fmt.Sprintf("App %03d", i), IconPNG: data}
	}
	f.state.Apps = map[string]domain.AppListing{"USB-DEMO-01": {Apps: apps}}
	u := New(f)
	u.deviceTab = "packages"
	s, err := headless.Open(u.Config(), nil, headless.Options{Scale: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if len(u.apps.bitmaps.entries) != 256 {
		t.Fatalf("expected 256 decoded icons, got %d", len(u.apps.bitmaps.entries))
	}
	e := u.apps.bitmaps.entries["com.example.app000"].Value.(appBitmapEntry)
	original := &e.bitmap.Pix[0]
	for _, width := range []int{1100, 1400, 1240} {
		if err := s.Resize(width, 820); err != nil {
			t.Fatal(err)
		}
	}
	e = u.apps.bitmaps.entries["com.example.app000"].Value.(appBitmapEntry)
	if original != &e.bitmap.Pix[0] {
		t.Fatal("window redraw decoded icons again")
	}
	clean(t, s)
}

func BenchmarkAppIconDecode256(b *testing.B) {
	data := bitmapTestPNG(b, color.NRGBA{R: 200, G: 100, A: 255}, 192)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for n := 0; n < 256; n++ {
			appIconBitmap("com.example", data)
		}
	}
}

func BenchmarkAppBitmapCached256(b *testing.B) {
	data := bitmapTestPNG(b, color.NRGBA{R: 200, G: 100, A: 255}, 192)
	var cache appBitmapCache
	keys := make([]string, 256)
	for i := range keys {
		keys[i] = fmt.Sprintf("phone\x00com.example.app%d", i)
		cache.bitmap(keys[i], data)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, key := range keys {
			cache.bitmap(key, data)
		}
	}
}
