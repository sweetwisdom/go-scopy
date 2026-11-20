package fileview

import (
	"bytes"
	"strings"
	"testing"
)

func TestPathsAndNames(t *testing.T) {
	for _, s := range []string{"", "relative", "/sdcard/a\x00b", "/sdcard/a\nb", "/a\rb"} {
		if _, err := CleanPath(s); err == nil {
			t.Errorf("accepted path %q", s)
		}
	}
	if p, err := CleanPath("/sdcard/DCIM/../Pictures//"); err != nil || p != "/sdcard/Pictures" {
		t.Fatalf("path = %q, %v", p, err)
	}
	for _, s := range []string{"", ".", "..", "a/b", "a\n", "a\x00"} {
		if ValidName(s) {
			t.Errorf("accepted name %q", s)
		}
	}
	for _, s := range []string{"中文 空格.txt", "a'$(id);.txt", "<img src=x onerror=alert(1)>.txt"} {
		if !ValidName(s) {
			t.Errorf("rejected safe leaf %q", s)
		}
	}
}

func TestPreviewPolicies(t *testing.T) {
	for _, name := range []string{"x.svg", "a.pdf", "a.docx", "image.exe", "unknown"} {
		if _, err := PreviewSpec(name); err == nil {
			t.Errorf("active/unsupported format %s accepted", name)
		}
	}
	spec, err := PreviewSpec("INDEX.HTML")
	if err != nil || spec.Kind != "text" || spec.MIME != "text/plain; charset=utf-8" {
		t.Fatalf("HTML policy: %+v %v", spec, err)
	}
	if err := ValidatePreview(spec, []byte("<script>alert(1)</script>")); err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{{0xff}, {0}, bytes.Repeat([]byte{'a'}, int(spec.Limit+1))} {
		if ValidatePreview(spec, data) == nil {
			t.Fatal("invalid/oversized text accepted")
		}
	}
	spec, _ = PreviewSpec("a.png")
	if ValidatePreview(spec, []byte("<html>not an image</html>")) == nil {
		t.Fatal("disguised HTML accepted as image")
	}
}

func TestResourceIsolationAndInvalidation(t *testing.T) {
	var r Resources
	for _, name := range []string{"index.html", "workspace.css", "workspace.js", "model.mjs"} {
		mime, data, ok := r.Serve(name)
		if !ok || len(data) == 0 || mime == "" {
			t.Fatalf("missing asset %q: %s", name, mime)
		}
	}
	for _, name := range []string{"../go.mod", "assets/../../go.mod", "C:/Windows/win.ini", "preview/not-a-token"} {
		if _, _, ok := r.Serve(name); ok {
			t.Errorf("served %q", name)
		}
	}
	spec, _ := PreviewSpec("a.txt")
	url, err := r.Put(spec, []byte("error: this is file content"))
	if err != nil {
		t.Fatal(err)
	}
	mime, data, ok := r.Serve(strings.TrimPrefix(url, "./"))
	if !ok || !bytes.HasPrefix(data, []byte("error:")) || !strings.HasPrefix(mime, "text/plain") {
		t.Fatalf("preview missing: %s %q %v", mime, data, ok)
	}
	url2, _ := r.Put(spec, []byte("second"))
	if url2 == url {
		t.Fatal("token reused")
	}
	if _, _, ok := r.Serve(strings.TrimPrefix(url, "./")); ok {
		t.Fatal("old preview survived replacement")
	}
	r.Clear()
	if _, _, ok := r.Serve(strings.TrimPrefix(url2, "./")); ok {
		t.Fatal("preview survived clear")
	}
}

func TestWorkspaceContentSecurityPolicy(t *testing.T) {
	_, html, _ := new(Resources).Serve("index.html")
	for _, fragment := range []string{"default-src 'none'", "script-src 'self'", "object-src 'none'", "base-uri 'none'"} {
		if !bytes.Contains(html, []byte(fragment)) {
			t.Errorf("missing CSP %s", fragment)
		}
	}
	if bytes.Contains(html, []byte("https://")) {
		t.Fatal("workspace unexpectedly depends on the network")
	}
}
