package desktop

import (
	"bytes"
	"container/list"

	"github.com/vivaui/viva/ui"
)

const appBitmapCacheLimit = 64 << 20
const appBitmapCacheEntries = 1024

type appBitmapEntry struct {
	key    string
	data   []byte // State-owned PNG bytes are immutable.
	bitmap ui.Bitmap
	valid  bool
}

// UI-thread-only LRU: identical snapshots retain the decoded pixels, including
// after a backend refresh publishes a new byte slice with unchanged content.
type appBitmapCache struct {
	entries map[string]*list.Element
	recent  list.List
	bytes   int
}

func (c *appBitmapCache) bitmap(key string, data []byte) (ui.Bitmap, bool) {
	if len(data) == 0 {
		return ui.Bitmap{}, false
	}
	if c.entries == nil {
		c.entries = map[string]*list.Element{}
	}
	if e := c.entries[key]; e != nil {
		entry := e.Value.(appBitmapEntry)
		if len(entry.data) == len(data) && (&entry.data[0] == &data[0] || bytes.Equal(entry.data, data)) {
			c.recent.MoveToFront(e)
			return entry.bitmap, entry.valid
		}
		c.remove(e)
	}
	bitmap, ok := appIconBitmap(key, data)
	size := len(data) + len(bitmap.Pix)
	if size > appBitmapCacheLimit {
		return bitmap, ok
	}
	for c.recent.Len() > 0 && (c.bytes+size > appBitmapCacheLimit || len(c.entries) >= appBitmapCacheEntries) {
		c.remove(c.recent.Back())
	}
	c.entries[key] = c.recent.PushFront(appBitmapEntry{key: key, data: data, bitmap: bitmap, valid: ok})
	c.bytes += size
	return bitmap, ok
}

func (c *appBitmapCache) remove(e *list.Element) {
	entry := e.Value.(appBitmapEntry)
	c.bytes -= len(entry.data) + len(entry.bitmap.Pix)
	delete(c.entries, entry.key)
	c.recent.Remove(e)
}
