package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/scriptstore"
	"github.com/go-scopy/go-scopy/internal/scriptview"
	"github.com/vivaui/viva/headless"
)

type scriptFixture struct {
	*fixture
	store   *scriptstore.Store
	runs    []domain.ScriptRun
	request domain.ScriptRequest
	stopped string
}

func (f *scriptFixture) ScriptFiles() (scriptstore.Listing, error) { return f.store.List() }
func (f *scriptFixture) OpenScriptFile(p string) (scriptstore.Document, error) {
	return f.store.Open(p)
}
func (f *scriptFixture) ReadScriptFile(p string) (scriptstore.Document, error) {
	return f.store.Read(p)
}
func (f *scriptFixture) NewScriptFile(n, c string) (scriptstore.Document, error) {
	return f.store.Create(n, c)
}
func (f *scriptFixture) SaveScriptFile(p, c, v string) (scriptstore.Document, error) {
	return f.store.Save(p, c, v)
}
func (f *scriptFixture) TrustScriptFile(path, version string, trusted bool) (scriptstore.Document, error) {
	return f.store.Trust(path, version, trusted)
}
func (f *scriptFixture) ScriptGuide() (string, error)    { return f.store.Guide(scriptview.Guide()) }
func (f *scriptFixture) ScriptState() domain.ScriptState { return domain.ScriptState{Runs: f.runs} }
func (f *scriptFixture) RunScript(r domain.ScriptRequest) (string, error) {
	f.request = r
	return "test-run", nil
}
func (f *scriptFixture) StopScript(id string) { f.stopped = id }
func newScriptFixture(t *testing.T) *scriptFixture {
	t.Helper()
	s, e := scriptstore.New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return &scriptFixture{fixture: sample(), store: s}
}
func scriptUI(t *testing.T) (*UI, *scriptFixture, *[]map[string]any) {
	f := newScriptFixture(t)
	u := New(f)
	u.section = "scripts"
	u.scriptWeb.requested = true
	messages := []map[string]any{}
	u.scriptWeb.post = func(raw string) {
		var m map[string]any
		if e := json.Unmarshal([]byte(raw), &m); e != nil {
			t.Fatal(e)
		}
		messages = append(messages, m)
	}
	u.scriptPageMessage(`{"action":"ready"}`)
	t.Cleanup(u.closeScriptWeb)
	return u, f, &messages
}
func sendScript(u *UI, m scriptMessage) { b, _ := json.Marshal(m); u.scriptPageMessage(string(b)) }
func TestScriptsRunOnlyTrustedKnownVersionAndOnlineDevice(t *testing.T) {
	u, f, msgs := scriptUI(t)
	doc, e := f.store.Create("safe.py", "print(1)")
	if e != nil {
		t.Fatal(e)
	}
	base := scriptMessage{ID: 1, Action: "run", Path: doc.Path, Version: doc.Version, Serial: u.selected, Trusted: true, Timeout: 30}
	cases := []scriptMessage{base, base, base, base, base}
	cases[0].Trusted = false
	cases[1].Serial = "USB-DEMO-03"
	cases[2].Serial = "offline"
	cases[3].Version = "stale"
	cases[4].Path = filepath.Join(t.TempDir(), "unapproved.py")
	for _, m := range cases {
		sendScript(u, m)
		if f.request.Path != "" {
			t.Fatal("started invalid request", m)
		}
		last := (*msgs)[len(*msgs)-1]
		if last["type"] != "response" || last["error"] == "" {
			t.Fatalf("invalid reply %+v", last)
		}
	}
	sendScript(u, base)
	if f.request.Serial != u.selected || f.request.Path != doc.Path {
		t.Fatalf("request %+v", f.request)
	}
	f.request = domain.ScriptRequest{}
	u.section = "devices"
	sendScript(u, base)
	if f.request.Path != "" {
		t.Fatal("hidden page started script")
	}
}
func TestScriptWireExposesOutputAndOnlyOnlineDevices(t *testing.T) {
	u, f, msgs := scriptUI(t)
	f.runs = []domain.ScriptRun{{ID: "run-1", Status: "running", Log: "真实日志 UTF-8"}}
	u.sendScriptContext()
	last := (*msgs)[len(*msgs)-1]
	if len(last["devices"].([]any)) != 2 {
		t.Fatal("unauthorized device exposed")
	}
	runs := last["runs"].([]any)
	if runs[0].(map[string]any)["log"] != "真实日志 UTF-8" {
		t.Fatal("log omitted by JSON tag")
	}
	sendScript(u, scriptMessage{ID: 1, Action: "stop", RunID: "forged"})
	if f.stopped != "" {
		t.Fatal("stopped unknown run")
	}
	sendScript(u, scriptMessage{ID: 2, Action: "stop", RunID: "run-1"})
	if f.stopped != "run-1" {
		t.Fatal("did not stop selected run")
	}
}
func TestGuideCallsSystemOpenerWithRealMDAndNoScriptExecution(t *testing.T) {
	u, f, _ := scriptUI(t)
	opened := ""
	u.scriptWeb.openPath = func(path string) error { opened = path; return nil }
	sendScript(u, scriptMessage{ID: 1, Action: "guide"})
	if !strings.HasSuffix(opened, ".md") {
		t.Fatal("not a Markdown file", opened)
	}
	b, e := os.ReadFile(opened)
	if e != nil || !strings.Contains(string(b), "goscrcpy.device_ids()") {
		t.Fatalf("guide %v", e)
	}
	if f.request.Path != "" {
		t.Fatal("guide executed a script")
	}
}
func TestScriptNavigationAndUnsavedClosePrompt(t *testing.T) {
	t.Setenv("VIVA_HEADLESS", "1")
	f := newScriptFixture(t)
	u := New(f)
	s, e := headless.Open(u.Config(), nil, headless.Options{Scale: 1})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	click(t, s, "脚本")
	if u.section != "scripts" {
		t.Fatal("missing standalone navigation")
	}
	clean(t, s)
	for _, tab := range devicePageKeys {
		if tab.key == "scripts" {
			t.Fatal("old device tab still present")
		}
	}
	called := false
	u.scriptWeb.dirty = true
	u.Config().OnCloseRequest(func() { called = true })
	if called || !u.closing {
		t.Fatal("unsaved script closed without warning")
	}
}

func TestScriptTrustRestoresAndExternalEditsRequireConfirmation(t *testing.T) {
	u, f, _ := scriptUI(t)
	doc, err := f.store.Create("remember.py", "print('one')\n")
	if err != nil {
		t.Fatal(err)
	}
	sendScript(u, scriptMessage{ID: 1, Action: "trust", Path: doc.Path, Version: doc.Version, Trusted: true})
	restored, err := f.store.Read(doc.Path)
	if err != nil || !restored.Trusted {
		t.Fatal("confirmation not persisted", err)
	}
	sendScript(u, scriptMessage{ID: 2, Action: "run", Path: doc.Path, Version: doc.Version, Serial: u.selected, Timeout: 30})
	if f.request.Path != doc.Path {
		t.Fatal("remembered trust did not authorize unchanged script")
	}
	f.request = domain.ScriptRequest{}
	if err := os.WriteFile(doc.Path, []byte("print('external edit')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	current, err := f.store.Read(doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	sendScript(u, scriptMessage{ID: 3, Action: "run", Path: current.Path, Version: current.Version, Serial: u.selected, Timeout: 30})
	if f.request.Path != "" {
		t.Fatal("external edit inherited saved trust")
	}
}

type scriptDependencyFixture struct {
	*scriptFixture
	dependencies                       *domain.ScriptDependencies
	dependencyAction, dependencyPython string
}

func (f *scriptDependencyFixture) ScriptState() domain.ScriptState {
	return domain.ScriptState{Runs: f.runs, Dependencies: f.dependencies}
}
func (f *scriptDependencyFixture) CheckScriptDependencies(python string) error {
	f.dependencyAction, f.dependencyPython = "check", python
	return nil
}
func (f *scriptDependencyFixture) InstallScriptDependencies(python string) error {
	f.dependencyAction, f.dependencyPython = "install", python
	return nil
}
func (f *scriptDependencyFixture) StopScriptDependencies() { f.dependencyAction = "stop" }

func TestScriptDependencyBridgeUsesChosenPythonAndRejectsHiddenPage(t *testing.T) {
	u, f, msgs := scriptUI(t)
	backend := &scriptDependencyFixture{scriptFixture: f, dependencies: &domain.ScriptDependencies{RequestedPython: "chosen-python", Python: "chosen-python", Operation: "install", Status: "running", Log: "pip output"}}
	u.backend = backend
	for _, action := range []string{"dependencies-check", "dependencies-install", "dependencies-stop"} {
		sendScript(u, scriptMessage{ID: 1, Action: action, Python: "chosen-python"})
		if backend.dependencyAction != strings.TrimPrefix(action, "dependencies-") {
			t.Fatal("wrong dependency action", backend.dependencyAction)
		}
	}
	if backend.dependencyPython != "chosen-python" {
		t.Fatal("selected Python ignored")
	}
	u.scriptWeb.contextSent = ""
	u.sendScriptContext()
	last := (*msgs)[len(*msgs)-1]
	deps := last["dependencies"].(map[string]any)
	if deps["log"] != "pip output" || last["dependencySupport"] != true {
		t.Fatal("environment output missing", last)
	}
	backend.dependencyAction = ""
	u.section = "devices"
	sendScript(u, scriptMessage{ID: 2, Action: "dependencies-install", Python: "other-python"})
	if backend.dependencyAction != "" {
		t.Fatal("hidden page installed dependencies")
	}
}
