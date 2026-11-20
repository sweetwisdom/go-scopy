// Package scriptview embeds the offline Python editor and its optional file API.
package scriptview

import (
	"embed"
	"io/fs"
	"mime"
	"path"
	"strings"

	"github.com/go-scopy/go-scopy/internal/scriptstore"
)

//go:embed assets/*
var assets embed.FS

func Serve(name string) (string, []byte, bool) {
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		name = "index.html"
	}
	if !fs.ValidPath(name) || strings.Contains(name, "/") {
		return "", nil, false
	}
	b, err := assets.ReadFile("assets/" + name)
	if err != nil {
		return "", nil, false
	}
	mimeType := mime.TypeByExtension(path.Ext(name))
	if mimeType == "" {
		mimeType = "text/plain; charset=utf-8"
	}
	return mimeType, b, true
}

// FileBackend never treats a browser-supplied path as permission to read host files.
// OpenScriptFile is reserved for the native picker; Read/Save require a known file.
func Guide() []byte { b, _ := assets.ReadFile("assets/python-development.md"); return b }

type FileBackend interface {
	ScriptGuide() (string, error)
	ScriptFiles() (scriptstore.Listing, error)
	OpenScriptFile(string) (scriptstore.Document, error)
	ReadScriptFile(string) (scriptstore.Document, error)
	NewScriptFile(string, string) (scriptstore.Document, error)
	SaveScriptFile(string, string, string) (scriptstore.Document, error)
}

// TrustBackend persists confirmation separately from the user-editable file.
type TrustBackend interface {
	TrustScriptFile(path, version string, trusted bool) (scriptstore.Document, error)
}
