package scriptview

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestOfflineEditorAndGuide(t *testing.T) {
	for _, name := range []string{"index.html", "workspace.js", "workspace.css", "licenses.txt", "python-development.md"} {
		mime, b, ok := Serve(name)
		if !ok || mime == "" || len(b) == 0 {
			t.Fatalf("missing %s", name)
		}
	}
	for _, name := range []string{"../src/main.js", "node_modules/codemirror/package.json", "file:///C:/test.py"} {
		if _, _, ok := Serve(name); ok {
			t.Fatalf("unsafe %s", name)
		}
	}
	_, html, _ := Serve("index.html")
	for _, text := range []string{"connect-src 'none'", "id=\"guide\"", "id=\"device\""} {
		if !strings.Contains(string(html), text) {
			t.Fatal("missing", text)
		}
	}
	source, e := os.ReadFile("../../docs/python-script-development.md")
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(source, Guide()) {
		t.Fatal("rebuild scriptview assets: embedded guide is stale")
	}
}
