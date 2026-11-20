package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-scopy/go-scopy/internal/automation"
	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/scriptview"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"
)

type liveScriptFixture struct {
	*scriptFixture
	manager *automation.Manager
}

func (f *liveScriptFixture) ScriptState() domain.ScriptState { return f.manager.Snapshot() }
func (f *liveScriptFixture) RunScript(r domain.ScriptRequest) (string, error) {
	return f.manager.Start(r)
}
func (f *liveScriptFixture) StopScript(id string) { f.manager.Stop(id) }

// Opt-in real WebView + CodeMirror + local Python + read-only ADB test.
// It neither initializes uiautomator2 on the phone nor changes apps/settings.
func TestScriptWorkspaceLiveWebView(t *testing.T) {
	if os.Getenv("GOSCOPY_LIVE_SCRIPTVIEW") != "1" {
		t.Skip("opt-in native script workspace test")
	}
	adb, serial, python := os.Getenv("GOSCOPY_LIVE_ADB"), os.Getenv("GOSCOPY_LIVE_SERIAL"), os.Getenv("GOSCOPY_LIVE_PYTHON")
	if adb == "" || serial == "" || python == "" {
		t.Fatal("set GOSCOPY_LIVE_ADB, GOSCOPY_LIVE_SERIAL and GOSCOPY_LIVE_PYTHON")
	}
	t.Setenv("GOSCRCPY_PYTHON", python)
	root, e := filepath.Abs("../..")
	if e != nil {
		t.Fatal(e)
	}
	f := &liveScriptFixture{scriptFixture: newScriptFixture(t)}
	f.state.Devices = []domain.Device{{Serial: serial, Model: "Script live test", State: "device", Connection: "Wi-Fi"}}
	u := New(f)
	u.section = "scripts"
	u.scriptWeb.requested = true
	f.manager = automation.New(root, t.TempDir(), adb, u.wake.Invalidate)
	defer f.manager.Close()
	var canvas ui.Canvas
	signals, finished := make(chan error, 1), make(chan error, 1)
	signal := func(err error) {
		select {
		case signals <- err:
		default:
		}
	}
	guideOpened := false
	u.scriptWeb.openPath = func(path string) error {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if filepath.Ext(path) != ".md" || len(b) < 1000 {
			return errors.New("invalid Markdown guide")
		}
		guideOpened = true
		return nil
	}
	err := app.RunApplication(func(application *app.Application) error {
		config := u.Config()
		config.Title = "Go-Scopy Script Integration"
		config.Class = "GoScopy.Script.Test"
		config.Web.OnReady = func() {
			u.fileWeb.hostReady = true
			page, err := u.fileWeb.host.Open(app.WebViewConfig{URL: "goscscripts://workspace/index.html", Scheme: &app.WebScheme{Name: "goscscripts", Serve: scriptview.Serve}, OnMessage: func(raw string) {
				var m struct {
					Action string
					OK     bool
					Error  string
					Stats  json.RawMessage
				}
				_ = json.Unmarshal([]byte(raw), &m)
				if m.Action == "_test_complete" {
					if !m.OK {
						signal(fmt.Errorf("native script page: %s", m.Error))
						return
					}
					if !guideOpened {
						signal(errors.New("guide was not passed to system opener"))
						return
					}
					if report := os.Getenv("GOSCOPY_SCRIPT_REPORT"); report != "" {
						if err := os.MkdirAll(filepath.Dir(report), 0700); err != nil {
							signal(err)
							return
						}
						if err := os.WriteFile(report, m.Stats, 0600); err != nil {
							signal(err)
							return
						}
					}
					signal(nil)
					return
				}
				u.scriptPageMessage(raw)
				if m.Action == "ready" {
					u.scriptWeb.page.Eval(scriptNativeProbe)
				}
			}})
			if err != nil {
				signal(err)
				return
			}
			u.scriptWeb.page = page
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
			case <-time.After(50 * time.Second):
				result = errors.New("native script test timed out")
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
	if !u.scriptWeb.closed {
		t.Fatal("programmatic close retained script page")
	}
}

const scriptNativeProbe = `(async()=>{
 const sleep=ms=>new Promise(r=>setTimeout(r,ms));
 const $=id=>document.getElementById(id);
 const wait=async(fn,label)=>{const end=Date.now()+22000;while(Date.now()<end){if(fn())return;await sleep(30)};throw Error(label+'; message='+$('message-region').textContent+'; log='+$('log').textContent)};
 const paste=text=>{const node=document.querySelector('.cm-content');node.focus();const data=new DataTransfer();data.setData('text/plain',text);node.dispatchEvent(new ClipboardEvent('paste',{clipboardData:data,bubbles:true,cancelable:true}))};
 try{
  await wait(()=>!$('new').disabled,'workspace not ready');
  $('new').click();await wait(()=>$('new-dialog').open,'new dialog');
  $('new-name').value='readonly_native.py';$('template').value='model';$('new-form').requestSubmit();
  await wait(()=>$('filename').textContent==='readonly_native.py'&&!$('save').disabled,'script creation');
  paste('print("GOSCOPY_SCRIPT_WEBVIEW_OK", flush=True)\n');
  await wait(()=>$('filename').textContent.includes('未保存'),'CodeMirror paste');
  $('save').click();await wait(()=>!$('filename').textContent.includes('未保存')&&!$('save').disabled,'save');
  $('trusted').click();await wait(()=>!$('run').disabled,'explicit device/trust');
  $('run').click();await wait(()=>$('log').textContent.includes('GOSCOPY_SCRIPT_WEBVIEW_OK'),'real Python output');
  await wait(()=>$('run-status').textContent.includes('已完成'),'read-only script exit');
  if(!$('log').textContent.includes('Model:'))throw Error('ADB getprop did not produce a model');
  const success=$('run-status').textContent;
  paste('import time\nprint("GOSCOPY_SCRIPT_STOP_READY", flush=True)\ntime.sleep(30)\n');
  await wait(()=>$('filename').textContent.includes('未保存'),'second edit');
  if($('trusted').checked)throw Error('editing retained trust');
  $('trusted').click();$('run').click();
  await wait(()=>$('log').textContent.includes('GOSCOPY_SCRIPT_STOP_READY'),'save-and-run');
  $('stop').click();await wait(()=>$('run-status').textContent.includes('已取消'),'stop local process');
  $('guide').click();await sleep(200);
  const r=$('editor').getBoundingClientRect();
  const stats={nativeWebView:true,codeMirrorPaste:true,localFileSave:true,explicitDevice:true,readOnlyADB:true,pythonSuccess:success,stop:true,guideSystemOpener:true,editor:{width:r.width,height:r.height},horizontalOverflow:document.documentElement.scrollWidth>innerWidth};
  if(stats.horizontalOverflow||r.width<100||r.height<100)throw Error('editor layout invalid');
  window.viva.postMessage(JSON.stringify({action:'_test_complete',ok:true,stats}));
 }catch(e){window.viva.postMessage(JSON.stringify({action:'_test_complete',ok:false,error:e.stack||e.message}))}
})()`
