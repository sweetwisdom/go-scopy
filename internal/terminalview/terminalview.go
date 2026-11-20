// Package terminalview embeds the offline xterm.js workspace and its narrow
// read-only completion contract. Browser input never selects host executables.
package terminalview

import (
	"context"
	"embed"
	"io/fs"
	"mime"
	"path"
	"strings"
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
	data, err := assets.ReadFile("assets/" + name)
	if err != nil {
		return "", nil, false
	}
	return mime.TypeByExtension(path.Ext(name)), data, true
}

// CompletionBackend is reserved for the deferred command-assistance work.
// The current xterm-only page does not request candidates. It only exposes fixed read-only device queries, not a general
// command execution API. It is optional, so synthetic/backwards-compatible
// backends still get static command/option completion.
type CompletionBackend interface {
	TerminalCandidates(context.Context, string, string) ([]string, error)
}
