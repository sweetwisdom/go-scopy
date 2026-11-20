package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/terminalview"
	"github.com/go-scopy/go-scopy/internal/tools"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"
)

type liveTerminalFixture struct {
	*fixture
	tool tools.Tools
}

func (f *liveTerminalFixture) OpenShell(serial string, cols, rows int, notify func()) (domain.Terminal, error) {
	s, err := f.tool.DeviceShell(serial, cols, rows, notify)
	if err != nil {
		return nil, err
	}
	return s, nil
}
func (f *liveTerminalFixture) OpenLogcat(serial string, cols, rows int, notify func()) (domain.Terminal, error) {
	s, err := f.tool.DeviceLogcat(serial, cols, rows, notify)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Opt-in real WebView2 + xterm.js + adb test. Only a printf marker is executed;
// Logcat is read without exporting its contents. No apps/files/settings change.
func TestTerminalWorkspaceLiveWebView(t *testing.T) {
	if os.Getenv("GOSCOPY_LIVE_TERMINALVIEW") != "1" {
		t.Skip("opt-in native terminal WebView test")
	}
	adb, serial := os.Getenv("GOSCOPY_LIVE_ADB"), os.Getenv("GOSCOPY_LIVE_SERIAL")
	if adb == "" || serial == "" {
		t.Fatal("set GOSCOPY_LIVE_ADB and GOSCOPY_LIVE_SERIAL")
	}
	f := &liveTerminalFixture{fixture: sample(), tool: tools.Tools{ADB: adb}}
	f.state.Devices = []domain.Device{{Serial: serial, Model: "Live xterm test", State: "device", Connection: "Wi-Fi"}}
	u := New(f)
	u.deviceTab = "terminal"
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
		config.Title = "Go-Scopy Terminal Integration"
		config.Class = "GoScopy.Terminal.Test"
		config.Web.OnReady = func() {
			u.fileWeb.hostReady = true
			w := &u.terminalWeb
			w.requested = true
			page, err := u.fileWeb.host.Open(app.WebViewConfig{
				URL: "goscterm://workspace/index.html", Scheme: &app.WebScheme{Name: "goscterm", Serve: terminalview.Serve},
				OnMessage: func(raw string) {
					var m struct {
						Action string
						OK     bool
						Error  string
						Stats  json.RawMessage
					}
					_ = json.Unmarshal([]byte(raw), &m)
					if m.Action == "_test_complete" {
						if m.OK {
							if output := os.Getenv("GOSCOPY_TERMINAL_REPORT"); output != "" {
								if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
									signal(err)
									return
								}
								if err := os.WriteFile(output, m.Stats, 0600); err != nil {
									signal(err)
									return
								}
							}
							signal(nil)
						} else {
							signal(fmt.Errorf("native terminal page: %s", m.Error))
						}
						return
					}
					u.terminalPageMessage(raw)
					if m.Action == "ready" {
						w.page.Eval(terminalNativeProbe)
					}
				},
			})
			if err != nil {
				signal(err)
				return
			}
			w.page = page
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
			case <-time.After(45 * time.Second):
				result = errors.New("native terminal WebView test timed out")
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
	if !u.terminalWeb.closed || len(u.terminals) != 0 {
		t.Fatal("programmatic window close retained terminal processes")
	}
}

const terminalNativeProbe = `(async()=>{
 const sleep=ms=>new Promise(r=>setTimeout(r,ms));
 const wait=async(fn,label)=>{const end=Date.now()+18000;while(Date.now()<end){if(fn())return;await sleep(30)};throw Error(label+'; toast='+(document.querySelector('.toast')?.textContent||'')+'; status='+(document.querySelector('.session-footer')?.textContent||'')+'; panes='+document.querySelectorAll('.session').length)};
 const pane=()=>document.querySelector('.session[data-mode="shell"]');
 const screen=()=>pane().querySelector('.xterm-screen');
 const text=()=>pane().querySelector('.xterm-rows')?.textContent||'';
 try {
  await wait(()=>pane()?.dataset.connected==='true','shell did not connect');
  await wait(()=>screen()?.clientWidth>100,'xterm was not fitted');
  const input=pane().querySelector('.xterm-helper-textarea');
  const paste=value=>{const data=new DataTransfer();data.setData('text/plain',value);input.dispatchEvent(new ClipboardEvent('paste',{clipboardData:data,bubbles:true,cancelable:true}))};
  paste("printf 'GOSCOPY_%s\\n' XTERM_NATIVE_OK\n");
  await wait(()=>text().includes('GOSCOPY_XTERM_NATIVE_OK'),'paste/ADB/output roundtrip failed');
  // Exercise a real xterm keydown Enter, not just direct bridge writes.
  paste("printf 'GOSCOPY_%s\\n' XTERM_ENTER_OK");
  input.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',code:'Enter',keyCode:13,which:13,bubbles:true,cancelable:true}));
  await wait(()=>text().includes('GOSCOPY_XTERM_ENTER_OK'),'Enter key did not delimit the pipe-based shell command');
  paste("sleep 20\n");await sleep(150);
  input.dispatchEvent(new KeyboardEvent('keydown',{key:'c',code:'KeyC',keyCode:67,which:67,ctrlKey:true,bubbles:true,cancelable:true}));
  await sleep(150);
  paste("printf 'GOSCOPY_%s\\n' INTERRUPT_OK\n");
  await wait(()=>text().includes('GOSCOPY_INTERRUPT_OK'),'Ctrl+C did not interrupt the remote shell command');
  const before={width:screen().clientWidth,rows:pane().querySelectorAll('.xterm-rows>div').length};
  const workspace=document.querySelector('.workspace');
  workspace.style.width='420px';workspace.style.height='250px';
  await wait(()=>screen().clientWidth<before.width && pane().querySelectorAll('.xterm-rows>div').length<before.rows,'FitAddon did not respond to width/height');
  const small={width:screen().clientWidth,rows:pane().querySelectorAll('.xterm-rows>div').length};
  const rect=screen().getBoundingClientRect(),container=pane().querySelector('.terminal-surface').getBoundingClientRect();
  if(rect.right>container.right+1 || rect.bottom>container.bottom+1)throw Error('terminal grid overflowed its pane');
  workspace.style.width='100%';workspace.style.height='100%';
  await wait(()=>screen().clientWidth>small.width,'FitAddon did not restore');
  document.querySelectorAll('[role="tab"]')[1].click();
  await wait(()=>document.querySelector('.session[data-mode="logcat"]')?.dataset.connected==='true','Logcat did not connect');
  await wait(()=>document.querySelector('.session[data-mode="logcat"] .xterm-rows')?.textContent.trim().length>8,'Logcat did not render output');
  document.querySelectorAll('[role="tab"]')[0].click();
  await wait(()=>pane().style.display!=='none','Shell was not restored');
  if(!text().includes('GOSCOPY_XTERM_ENTER_OK'))throw Error('history was lost switching modes');
  document.querySelectorAll('.tool')[1].click();await sleep(100);
  if(!document.querySelector('.commands'))throw Error('command reference did not open');
  document.querySelector('.commands .close').click();await sleep(100);
  if(document.documentElement.scrollWidth>innerWidth)throw Error('workspace horizontal overflow');
  window.viva.postMessage(JSON.stringify({action:'_test_complete',ok:true,stats:{roundtrip:true,enter:true,interrupt:true,logcat:true,history:true,fitBefore:before,fitSmall:small,width:innerWidth,height:innerHeight}}));
 }catch(error){window.viva.postMessage(JSON.stringify({action:'_test_complete',ok:false,error:error.message}))}
})();`
