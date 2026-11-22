package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/vivaui/viva/headless"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/fileview"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"
)

type fileFixture struct {
	*fixture
	ops     []fileview.Operation
	entries []domain.FileEntry
}

func (f *fileFixture) BrowseFiles(ctx context.Context, serial, dir string) ([]domain.FileEntry, error) {
	if dir == "/slow" {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return append([]domain.FileEntry(nil), f.entries...), nil
}
func (f *fileFixture) ReadFilePreview(ctx context.Context, serial, remote string, limit int64) ([]byte, error) {
	return []byte("<script>not executed</script>\n来自 Go 消息桥的 UTF-8 文本。"), nil
}
func (f *fileFixture) FileTask(serial string, op fileview.Operation) uint64 {
	f.ops = append(f.ops, op)
	return uint64(len(f.ops))
}

func fileUI(t *testing.T) (*UI, *fileFixture) {
	t.Helper()
	f := &fileFixture{fixture: sample(), entries: []domain.FileEntry{{Name: "notes.txt", Size: 64}, {Name: "Documents", IsDir: true}, {Name: "..", IsDir: true}}}
	u := New(f)
	u.initFileWeb()
	u.deviceTab = "files"
	u.fileWeb.serial = u.selected
	u.fileWeb.dir = "/sdcard"
	u.fileWeb.entries = f.entries
	u.fileWeb.listed = true
	t.Cleanup(u.closeFileWeb)
	return u, f
}
func sendFileRequest(u *UI, m fileMessage) {
	data, _ := json.Marshal(m)
	u.filePageMessage(string(data))
}
func awaitFileRead(t *testing.T, u *UI) fileResult {
	t.Helper()
	select {
	case r := <-u.fileWeb.results:
		return r
	case <-time.After(2 * time.Second):
		t.Fatal("file read did not complete")
		return fileResult{}
	}
}

func TestFileMessagesCannotTargetAnotherDeviceOrUnlistedPath(t *testing.T) {
	u, f := fileUI(t)
	for _, m := range []fileMessage{
		{ID: 1, Action: "mkdir", Serial: "another-device", Path: "/sdcard", Name: "unsafe"},
		{ID: 2, Action: "rename", Serial: u.selected, Path: "/sdcard", Name: "notes.txt", Target: "../escape"},
		{ID: 3, Action: "delete", Serial: u.selected, Path: "/sdcard", Names: []string{".."}, Confirmed: true},
		{ID: 4, Action: "delete", Serial: u.selected, Path: "/sdcard", Names: []string{"notes.txt"}},
		{ID: 5, Action: "delete", Serial: u.selected, Path: "/different", Names: []string{"notes.txt"}, Confirmed: true},
	} {
		sendFileRequest(u, m)
	}
	if len(f.ops) != 0 {
		t.Fatalf("unsafe operations reached backend: %+v", f.ops)
	}
	sendFileRequest(u, fileMessage{ID: 6, Action: "delete", Serial: u.selected, Path: "/sdcard", Names: []string{"Documents"}, Confirmed: true})
	if len(f.ops) != 1 || !f.ops[0].Recursive || f.ops[0].Path != "/sdcard/Documents" {
		t.Fatalf("wrong operation: %+v", f.ops)
	}
}

func TestFileReadCancellationAndStaleResults(t *testing.T) {
	u, _ := fileUI(t)
	sendFileRequest(u, fileMessage{ID: 1, Action: "list", Serial: u.selected, Path: "/slow"})
	old := u.fileWeb.browseVersion
	sendFileRequest(u, fileMessage{ID: 2, Action: "list", Serial: u.selected, Path: "/sdcard"})
	if u.fileWeb.browseVersion == old {
		t.Fatal("navigation did not advance generation")
	}
	for n := 0; n < 2; n++ {
		r := awaitFileRead(t, u)
		u.filePageResult(r)
	}
	if !u.fileWeb.listed || len(u.fileWeb.entries) != 3 {
		t.Fatal("stale response replaced current directory")
	}
	sendFileRequest(u, fileMessage{ID: 3, Action: "preview", Serial: u.selected, Path: "/sdcard", Name: "notes.txt"})
	r := awaitFileRead(t, u)
	u.filePageResult(r)
	if _, _, ok := u.fileWeb.resources.Serve("preview/not-a-token"); ok {
		t.Fatal("preview accessible without its token")
	}
	sendFileRequest(u, fileMessage{ID: 4, Action: "cancelPreview", Serial: u.selected, Path: "/sdcard"})
	if u.fileWeb.previewVersion == r.generation {
		t.Fatal("cancel did not invalidate preview")
	}
}

func TestFileActionsRequireSuccessfulListingAndBackendErrorsStayErrors(t *testing.T) {
	u, f := fileUI(t)
	u.fileWeb.listed = false
	sendFileRequest(u, fileMessage{ID: 1, Action: "mkdir", Serial: u.selected, Path: "/sdcard", Name: "new"})
	if len(f.ops) != 0 {
		t.Fatal("operation ran without a successful directory read")
	}
	u.filePageResult(fileResult{message: fileMessage{ID: 2, Action: "list", Serial: u.selected, Path: "/sdcard"}, generation: u.fileWeb.browseVersion, err: errors.New("permission denied")})
	if u.fileWeb.listed {
		t.Fatal("failed listing marked ready")
	}
}

// Opt-in: exercises the real Viva/WebView2 message envelope and custom scheme,
// using inert fixtures, never modifying or exposing a user's Android files.
func TestFileWorkspaceLiveWebView(t *testing.T) {
	if os.Getenv("GOSCOPY_LIVE_FILEVIEW") != "1" {
		t.Skip("opt-in native WebView test")
	}
	f := &fileFixture{fixture: sample(), entries: []domain.FileEntry{{Name: "notes.txt", Size: 64, ModTime: "2026-11-20 10:00"}, {Name: "Documents", IsDir: true}}}
	u := New(f)
	u.deviceTab = "files"
	var canvas ui.Canvas
	signals, finished := make(chan error, 1), make(chan error, 1)
	signal := func(err error) {
		select {
		case signals <- err:
		default:
		}
	}
	err := app.RunApplication(func(application *app.Application) error {
		config := u.Config()
		config.Title = "Go-Scopy File Workspace Integration"
		config.Class = "GoScopy.FileWorkspace.Test"
		// Use the real host and page protocol, but intercept a test-only completion
		// message. The normal ready/list/preview traffic goes to the actual adapter.
		config.Web.OnReady = func() {
			w := &u.fileWeb
			w.hostReady = true
			w.requested = true
			page, err := w.host.Open(app.WebViewConfig{URL: "goscfiles://workspace/index.html", Scheme: &app.WebScheme{Name: "goscfiles", Serve: w.resources.Serve}, OnMessage: func(raw string) {
				var m struct {
					Action string
					OK     bool
					Error  string
				}
				_ = json.Unmarshal([]byte(raw), &m)
				if m.Action == "_test_complete" {
					if m.OK {
						signal(nil)
					} else {
						signal(fmt.Errorf("native page: %s", m.Error))
					}
					return
				}
				u.filePageMessage(raw)
			}})
			if err != nil {
				signal(err)
				return
			}
			w.page = page
		}
		originalWake := config.OnWake
		started := false
		config.OnWake = func() {
			originalWake()
			if u.fileWeb.listed && !started {
				started = true
				u.fileWeb.page.Eval(`(function(){
      var row=document.querySelector('.file[data-name="notes.txt"]');
      if(!row){window.viva.postMessage(JSON.stringify({action:'_test_complete',ok:false,error:'Go listing did not reach DOM'}));return;}
      row.dispatchEvent(new MouseEvent('dblclick',{bubbles:true}));
      var until=Date.now()+10000;
      var timer=setInterval(function(){
       var code=document.querySelector('.preview-code');
       if(code && code.textContent.indexOf('来自 Go 消息桥')>=0){clearInterval(timer);window.viva.postMessage(JSON.stringify({action:'_test_complete',ok:true}));}
       else if(Date.now()>until){clearInterval(timer);window.viva.postMessage(JSON.stringify({action:'_test_complete',ok:false,error:document.querySelector('#preview-content').textContent}));}
      },50);
    })();`)
			}
		}
		window, err := application.OpenWindow(config, &canvas)
		if err != nil {
			return err
		}
		window.Hide()
		go func() {
			var result error
			select {
			case result = <-signals:
			case <-time.After(30 * time.Second):
				result = errors.New("native WebView did not finish its bridge test")
			}
			finished <- result
			window.Close()
		}()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestFileNativeUploadPickersKeepAllSelectedPaths(t *testing.T) {
	for _, folder := range []bool{false, true} {
		u, f := fileUI(t)
		session, err := headless.Open(u.Config(), nil, headless.Options{Scale: 1})
		if err != nil {
			t.Fatal(err)
		}
		action := "upload"
		paths := []string{filepath.Join(t.TempDir(), "one.txt"), filepath.Join(t.TempDir(), "two.txt")}
		if folder {
			action = "uploadFolder"
			paths = []string{t.TempDir()}
			err = session.Driver().QueueOpenDirectory(paths[0], nil)
		} else {
			err = session.Driver().QueueOpenFiles(paths, nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		sendFileRequest(u, fileMessage{ID: 50, Action: action, Serial: u.selected, Path: "/sdcard", Confirmed: true})
		if err = session.Driver().Pump(); err != nil {
			t.Fatal(err)
		}
		if !folder {
			if request, ok := session.Driver().LastOpenFilesRequest(); !ok || !request.AllowMultiple {
				t.Fatal("native upload picker did not enable multi-selection")
			}
		}
		if len(f.ops) != len(paths) {
			t.Fatalf("lost selected paths: %+v", f.ops)
		}
		for i, op := range f.ops {
			if op.Target != paths[i] || op.Path != "/sdcard/"+filepath.Base(paths[i]) {
				t.Fatalf("wrong upload destination: %+v", op)
			}
		}
		session.Close()
	}
}
