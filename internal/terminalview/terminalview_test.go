package terminalview

import (
	"strings"
	"testing"
)

func TestOfflineResourcesAreBoundedToEmbeddedAssets(t *testing.T) {
	for _, name := range []string{"index.html", "workspace.js", "workspace.css"} {
		mime, data, ok := Serve(name)
		if !ok || mime == "" || len(data) == 0 {
			t.Fatalf("missing embedded asset %s", name)
		}
	}
	for _, name := range []string{"../src/main.js", "assets/index.html", "/../package.json", "file:///etc/passwd", "vendor/adb.js", "node_modules/vue/package.json"} {
		_, _, ok := Serve(name)
		if ok {
			t.Fatalf("untrusted resource escaped asset root: %s", name)
		}
	}
	_, html, _ := Serve("index.html")
	if !strings.Contains(string(html), "connect-src 'none'") {
		t.Fatal("offline page can open a network connection")
	}
	_, js, _ := Serve("workspace.js")
	if strings.Contains(string(js), "CodeMirror") || strings.Contains(string(js), "codemirror") {
		t.Fatal("deferred command editor entered production bundle")
	}
}
