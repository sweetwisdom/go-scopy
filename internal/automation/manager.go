// Package automation runs trusted Python scripts outside the desktop process.
package automation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
)

const maxTail = 32 << 10
const maxLog = 8 << 20
const historyLimit = 40

type liveRun struct {
	run        domain.ScriptRun
	cancel     context.CancelFunc
	tail       []byte
	size       int
	file       *os.File
	logError   error
	truncated  bool
	lastNotify time.Time
}

type Manager struct {
	mu                                       sync.Mutex
	dependency                               *liveRun
	dependencyRequested, dependencyOperation string
	dependencyReady                          bool
	root, dir, adb                           string
	runs                                     []*liveRun
	last                                     domain.ScriptRequest
	problem                                  string
	closed                                   bool
	notify                                   func()
	wg                                       sync.WaitGroup
}

func New(root, dataDir, adb string, notify func()) *Manager {
	// Child processes change cwd to the script directory. All runtime, SDK and
	// history paths must therefore be absolute even when CLI flags use ".".
	if value, err := filepath.Abs(root); err == nil {
		root = value
	}
	if value, err := filepath.Abs(dataDir); err == nil {
		dataDir = value
	}
	m := &Manager{root: root, dir: filepath.Join(dataDir, "automation"), adb: adb, notify: notify}
	m.load()
	return m
}

func (m *Manager) wake() {
	if m.notify != nil {
		m.notify()
	}
}

func (m *Manager) Snapshot() domain.ScriptState {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := domain.ScriptState{Last: m.last, Error: m.problem}
	if l := m.dependency; l != nil {
		s.Dependencies = &domain.ScriptDependencies{RequestedPython: m.dependencyRequested,
			Python: l.run.Request.Python, Operation: m.dependencyOperation,
			Status: l.run.Status, Ready: m.dependencyReady, Error: l.run.Error,
			Log: strings.ToValidUTF8(string(l.tail), "�")}
	}
	s.Last.Args = append([]string(nil), s.Last.Args...)
	for _, l := range m.runs {
		r := l.run
		r.Request.Args = append([]string(nil), r.Request.Args...)
		r.Log = strings.ToValidUTF8(string(l.tail), "�")
		s.Runs = append(s.Runs, r)
	}
	return s
}

func (m *Manager) Start(req domain.ScriptRequest) (string, error) {
	req.Serial = strings.TrimSpace(req.Serial)
	req.Python = strings.TrimSpace(req.Python)
	if req.Serial == "" {
		return "", errors.New("必须明确选择目标设备")
	}
	if req.TimeoutSeconds == 0 {
		req.TimeoutSeconds = 600
	}
	if req.TimeoutSeconds < 1 || req.TimeoutSeconds > 86400 {
		return "", errors.New("超时应为 1–86400 秒")
	}
	path, err := filepath.Abs(strings.TrimSpace(req.Path))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(path), ".py") {
		return "", errors.New("请选择普通 .py 脚本文件")
	}
	req.Path = path
	req.Args = append([]string(nil), req.Args...)
	var nonce [8]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return "", err
	}
	id := time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(nonce[:])
	runDir := filepath.Join(m.dir, "runs", id)

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return "", errors.New("脚本运行器已关闭")
	}
	if m.dependency != nil && m.dependencyOperation == "install" && m.dependency.run.Active() {
		m.mu.Unlock()
		return "", errors.New("正在安装 Python 依赖，请完成或停止安装后再运行脚本")
	}
	for _, l := range m.runs {
		if l.run.Active() && l.run.Request.Serial == req.Serial {
			m.mu.Unlock()
			return "", errors.New("该设备已有运行中的脚本，请先停止或等待完成")
		}
	}
	if err = os.MkdirAll(runDir, 0700); err != nil {
		m.mu.Unlock()
		return "", err
	}
	logPath := filepath.Join(runDir, "output.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		m.mu.Unlock()
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.TimeoutSeconds)*time.Second)
	l := &liveRun{run: domain.ScriptRun{ID: id, Request: req, Status: "running", Started: time.Now(), ExitCode: -1, LogPath: logPath, OutputDir: runDir}, cancel: cancel, file: logFile}
	if err = writeJSON(filepath.Join(runDir, "run.json"), l.run); err != nil {
		cancel()
		logFile.Close()
		m.mu.Unlock()
		return "", err
	}
	m.runs = append([]*liveRun{l}, m.runs...)
	m.last = req
	if err = writeJSON(filepath.Join(m.dir, "preferences.json"), req); err != nil {
		m.problem = "保存脚本偏好失败：" + err.Error()
	}
	m.trim()
	m.wg.Add(1)
	m.mu.Unlock()
	m.wake()
	go m.execute(ctx, l)
	return id, nil
}

func (m *Manager) execute(ctx context.Context, l *liveRun) {
	defer m.wg.Done()
	defer l.cancel()
	w := runWriter{m: m, l: l}
	req := l.run.Request
	python, lib, err := resolve(m.root, req.Python)
	exit := -1
	if err == nil {
		fmt.Fprintf(w, "[runner] device=%s\n[runner] python=%s\n[runner] script=%s\n", req.Serial, python, req.Path)
		bootstrap := `import importlib.metadata as m,runpy,sys
try:
    v=m.version("uiautomator2")
except m.PackageNotFoundError:
    sys.exit("当前 Python 未安装 uiautomator2；请使用该解释器执行 -m pip install --require-hashes -r tools/python/requirements.lock")
print("[runner] uiautomator2="+v,flush=True)
if v!="3.7.1":
    sys.exit("需要 uiautomator2==3.7.1；请在当前 Python 中安装 requirements.lock 的锁定依赖")
import goscrcpy
p=sys.argv.pop(1)
sys.argv[0]=p
sys.path.insert(0,__import__("os").path.dirname(p))
runpy.run_path(p,run_name="__main__")`
		args := append([]string{"-u", "-c", bootstrap, req.Path}, req.Args...)
		cmd := exec.CommandContext(ctx, python, args...)
		cmd.Dir = filepath.Dir(req.Path)
		cmd.Env = environment(lib, m.adb, req, l.run)
		cmd.Stdout, cmd.Stderr = w, w
		cmd.WaitDelay = 2 * time.Second
		exit, err = runProcess(ctx, cmd)
	}
	status, message := "success", ""
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		status = "timeout"
		message = "脚本执行超时，已终止桌面进程树"
	case errors.Is(ctx.Err(), context.Canceled):
		status = "cancelled"
		message = "脚本已取消"
	case err != nil:
		status = "failed"
		message = err.Error()
	}
	fmt.Fprintf(w, "\n[runner] status=%s exit=%d\n", status, exit)
	m.mu.Lock()
	l.run.ExitCode = exit
	l.run.Finished = time.Now()
	l.run.Status = status
	l.run.Error = message
	if l.logError != nil {
		l.run.Error += "; 日志写入失败：" + l.logError.Error()
		if l.run.Status == "success" {
			l.run.Status = "failed"
		}
	}
	if e := l.file.Close(); e != nil {
		m.problem = "关闭脚本日志失败：" + e.Error()
	}
	l.file = nil
	if e := writeJSON(filepath.Join(l.run.OutputDir, "run.json"), l.run); e != nil {
		m.problem = "保存执行记录失败：" + e.Error()
	}
	m.trim()
	m.mu.Unlock()
	m.wake()
}

type runWriter struct {
	m *Manager
	l *liveRun
}

func (w runWriter) Write(p []byte) (int, error) {
	m, l := w.m, w.l
	m.mu.Lock()
	l.tail = append(l.tail, p...)
	if len(l.tail) > maxTail {
		l.tail = append([]byte(nil), l.tail[len(l.tail)-maxTail:]...)
	}
	if l.file != nil && l.logError == nil {
		n := min(len(p), maxLog-l.size)
		if n > 0 {
			_, l.logError = l.file.Write(p[:n])
			l.size += n
		}
		if n < len(p) && !l.truncated {
			l.truncated = true
			_, e := io.WriteString(l.file, "\n[runner] log limit reached; further output is shown only in the live tail\n")
			if e != nil {
				l.logError = e
			}
		}
	}
	repaint := time.Since(l.lastNotify) >= 100*time.Millisecond
	if repaint {
		l.lastNotify = time.Now()
	}
	m.mu.Unlock()
	if repaint {
		m.wake()
	}
	return len(p), nil
}

func (m *Manager) Stop(id string) {
	m.mu.Lock()
	for _, l := range m.runs {
		if l.run.ID == id && l.run.Active() {
			l.run.Status = "stopping"
			l.cancel()
			break
		}
	}
	m.mu.Unlock()
	m.wake()
}
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	if m.dependency != nil && m.dependency.run.Active() {
		m.dependency.run.Status = "stopping"
		m.dependency.cancel()
	}
	for _, l := range m.runs {
		if l.run.Active() {
			l.run.Status = "stopping"
			l.cancel()
		}
	}
	m.mu.Unlock()
	m.wg.Wait()
}
func (m *Manager) trim() {
	for i := len(m.runs) - 1; i >= historyLimit; i-- {
		if !m.runs[i].run.Active() {
			m.runs = append(m.runs[:i], m.runs[i+1:]...)
		}
	}
}
func (m *Manager) load() {
	b, err := os.ReadFile(filepath.Join(m.dir, "preferences.json"))
	if err == nil {
		if e := json.Unmarshal(b, &m.last); e != nil {
			m.problem = "脚本偏好无法读取：" + e.Error()
		}
	}
	paths, _ := filepath.Glob(filepath.Join(m.dir, "runs", "*", "run.json"))
	// Sort first, then load only the most recent bounded set.
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	for _, path := range paths[:min(len(paths), historyLimit)] {
		var r domain.ScriptRun
		b, e := os.ReadFile(path)
		if e != nil || json.Unmarshal(b, &r) != nil {
			continue
		}
		if r.ID != filepath.Base(filepath.Dir(path)) {
			continue
		}
		r.OutputDir = filepath.Dir(path)
		r.LogPath = filepath.Join(r.OutputDir, "output.log")
		if r.Active() {
			r.Status = "interrupted"
			r.Error = "上次应用退出前未完成"
			r.Finished = time.Now()
			if e = writeJSON(path, r); e != nil {
				m.problem = e.Error()
			}
		}
		l := &liveRun{run: r}
		if f, e := os.Open(r.LogPath); e == nil {
			if st, e := f.Stat(); e == nil {
				f.Seek(max(0, st.Size()-maxTail), io.SeekStart)
				l.tail, _ = io.ReadAll(io.LimitReader(f, maxTail))
			}
			f.Close()
		}
		m.runs = append(m.runs, l)
	}
	sort.Slice(m.runs, func(i, j int) bool { return m.runs[i].run.Started.After(m.runs[j].run.Started) })
}
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".record-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func resolve(root, custom string) (string, string, error) {
	bases := []string{filepath.Join(root, "tools", "python"), filepath.Join(root, "..", "Resources", "tools", "python"), filepath.Join(root, "resources", "python")}
	if cwd, e := os.Getwd(); e == nil {
		bases = append(bases, filepath.Join(cwd, "resources", "python"))
	}
	var lib string
	for _, base := range bases {
		if st, e := os.Stat(filepath.Join(base, "lib", "goscrcpy", "__init__.py")); e == nil && !st.IsDir() {
			lib = filepath.Join(base, "lib")
			break
		}
	}
	if lib == "" {
		return "", "", errors.New("找不到脚本 SDK：resources/python/lib/goscrcpy")
	}
	if custom == "" {
		custom = os.Getenv("GOSCRCPY_PYTHON")
	}
	if custom != "" {
		p, e := exec.LookPath(custom)
		if e != nil {
			return "", "", fmt.Errorf("Python 不可用：%w", e)
		}
		p, e = filepath.Abs(p)
		return p, lib, e
	}
	// Prefer the platform's usual Python command, without loading a stale
	// bundled runtime manifest left over from an earlier installation.
	candidates := []string{"python3", "python"}
	if runtime.GOOS == "windows" {
		candidates = []string{"python", "python3"}
	}
	for _, name := range candidates {
		if p, e := exec.LookPath(name); e == nil {
			p, e = filepath.Abs(p)
			return p, lib, e
		}
	}
	return "", "", errors.New("PATH 中未找到 Python，请安装 Python 并加入 PATH，或手动指定解释器")
}

func environment(lib, adb string, req domain.ScriptRequest, r domain.ScriptRun) []string {
	values := map[string]string{}
	for _, v := range os.Environ() {
		k, x, ok := strings.Cut(v, "=")
		if ok {
			if runtime.GOOS == "windows" {
				k = strings.ToUpper(k)
			}
			values[k] = x
		}
	}
	// Resolve the interpreter independently of host PYTHONHOME. Keep its own
	// site-packages and user PYTHONPATH; never inject stale bundled wheels.
	for _, key := range []string{"PYTHONHOME", "VIRTUAL_ENV", "PYTHONSTARTUP"} {
		delete(values, key)
	}
	if hostPath := values["PYTHONPATH"]; hostPath != "" {
		values["PYTHONPATH"] = lib + string(os.PathListSeparator) + hostPath
	} else {
		values["PYTHONPATH"] = lib
	}
	values["PYTHONUNBUFFERED"] = "1"
	values["PYTHONIOENCODING"] = "utf-8"
	values["PYTHONUTF8"] = "1"
	values["GOSCRCPY_DEVICE_ID"] = req.Serial
	values["ANDROID_SERIAL"] = req.Serial
	values["ANDROID_DEVICE_ADDR"] = req.Serial
	ids, _ := json.Marshal([]string{req.Serial})
	values["GOSCRCPY_DEVICE_IDS"] = string(ids)
	values["GOSCRCPY_ADB_PATH"] = adb
	values["ADBUTILS_ADB_PATH"] = adb
	values["ANDROID_ADB_SERVER_PORT"] = "5037"
	values["ADB_SERVER_SOCKET"] = "tcp:127.0.0.1:5037"
	values["GOSCRCPY_RUN_ID"] = r.ID
	values["GOSCRCPY_RUN_DIR"] = r.OutputDir
	values["PATH"] = filepath.Dir(adb) + string(os.PathListSeparator) + values["PATH"]
	env := make([]string, 0, len(values))
	for k, v := range values {
		env = append(env, k+"="+v)
	}
	sort.Strings(env)
	return env
}
