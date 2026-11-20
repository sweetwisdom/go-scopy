package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
)

const dependencyIndex = "https://pypi.tuna.tsinghua.edu.cn/simple"

// Dependencies runs checks/installation outside the UI thread. The existing
// process-tree owner and bounded log writer also cover pip and its children.
func (m *Manager) Dependencies(python string, install bool) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("脚本运行器已关闭")
	}
	if m.dependency != nil && m.dependency.run.Active() {
		m.mu.Unlock()
		return errors.New("Python 环境任务正在进行，请先等待或停止")
	}
	if install {
		for _, run := range m.runs {
			if run.run.Active() {
				m.mu.Unlock()
				return errors.New("请先停止正在执行的脚本，再安装依赖")
			}
		}
	}
	timeout := time.Minute
	operation := "check"
	if install {
		timeout = 15 * time.Minute
		operation = "install"
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	l := &liveRun{run: domain.ScriptRun{Status: "running", Started: time.Now(), ExitCode: -1,
		Request: domain.ScriptRequest{Python: strings.TrimSpace(python)}}, cancel: cancel}
	m.dependency, m.dependencyReady = l, false
	m.dependencyRequested, m.dependencyOperation = strings.TrimSpace(python), operation
	m.wg.Add(1)
	m.mu.Unlock()
	m.wake()
	go m.executeDependencies(ctx, l, install)
	return nil
}

func (m *Manager) StopDependencies() {
	m.mu.Lock()
	if l := m.dependency; l != nil && l.run.Active() {
		l.run.Status = "stopping"
		l.cancel()
	}
	m.mu.Unlock()
	m.wake()
}

// The bundled lock is a simple, fully pinned requirements file. Refuse a missing,
// empty or unpinned lock rather than guessing versions or silently running pip.
func dependencyPins(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("无法读取锁定依赖清单：%w", err)
	}
	pins := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "--hash=") {
			continue
		}
		pair := strings.Fields(line)
		name, version, ok := strings.Cut(pair[0], "==")
		if !ok || name == "" || version == "" || strings.ContainsAny(name+version, ";<>~=!*@/") {
			return nil, errors.New("依赖清单包含未锁定条目")
		}
		if _, exists := pins[name]; exists {
			return nil, errors.New("依赖清单包含重复条目")
		}
		pins[name] = version
	}
	if pins["uiautomator2"] != "3.7.1" {
		return nil, errors.New("依赖清单必须锁定 uiautomator2==3.7.1")
	}
	return pins, nil
}

const dependencyCheck = `# GOSCOPY_DEPENDENCY_CHECK
import importlib.metadata as metadata, json, sys
issues = []
for name, wanted in json.loads(sys.argv[1]).items():
    try:
        actual = metadata.version(name)
        if actual != wanted:
            issues.append(name + ": " + actual + " (需要 " + wanted + ")")
    except metadata.PackageNotFoundError:
        issues.append(name + ": 未安装")
if issues:
    print("\n".join(issues), flush=True)
    sys.exit("Python 依赖缺失或版本不匹配，请点击安装依赖")
import uiautomator2, goscrcpy
print("[environment] 锁定依赖检查通过", flush=True)
`

func (m *Manager) executeDependencies(ctx context.Context, l *liveRun, install bool) {
	defer m.wg.Done()
	defer l.cancel()
	w := runWriter{m: m, l: l}
	python, lib, err := resolve(m.root, l.run.Request.Python)
	exit := -1
	if err == nil {
		m.mu.Lock()
		l.run.Request.Python = python
		m.mu.Unlock()
		lock := filepath.Join(filepath.Dir(lib), "requirements.lock")
		var pins map[string]string
		pins, err = dependencyPins(lock)
		if err == nil {
			fmt.Fprintf(w, "[environment] Python: %s\n[environment] 依赖清单: %s\n", python, lock)
			// No shell, user-provided flags, elevation, fallback index or --user.
			// The user explicitly chose this interpreter and installation action.
			run := func(args []string) (int, error) {
				cmd := exec.CommandContext(ctx, python, args...)
				cmd.Dir = filepath.Dir(lock)
				cmd.Env = environment(lib, m.adb, domain.ScriptRequest{}, domain.ScriptRun{})
				cmd.Stdout, cmd.Stderr = w, w
				cmd.Stdin = nil
				cmd.WaitDelay = 2 * time.Second
				return runProcess(ctx, cmd)
			}
			if install {
				args := []string{"-u", "-m", "pip", "install", "--require-hashes", "-r", lock, "-i", dependencyIndex, "--disable-pip-version-check", "--no-input"}
				fmt.Fprintf(w, "[environment] %q %q\n", python, args)
				exit, err = run(args)
				if err != nil {
					fmt.Fprintln(w, "[environment] 安装未完成。若提示 No module named pip，请先为此解释器准备 pip；应用不会自动提权、改换解释器或绕过哈希校验。")
				}
			}
			if err == nil {
				data, _ := json.Marshal(pins)
				exit, err = run([]string{"-u", "-c", dependencyCheck, string(data)})
			}
		}
	}
	status, message := "success", ""
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		status, message = "timeout", "Python 环境任务超时，已停止本地进程树"
	case errors.Is(ctx.Err(), context.Canceled):
		status, message = "cancelled", "Python 环境任务已停止；已安装的依赖不会自动回滚"
	case err != nil:
		status, message = "failed", err.Error()
	}
	if err != nil {
		fmt.Fprintf(w, "[environment] %v\n", err)
	}
	m.mu.Lock()
	l.run.Status, l.run.Error, l.run.ExitCode, l.run.Finished = status, message, exit, time.Now()
	m.dependencyReady = status == "success"
	m.mu.Unlock()
	m.wake()
}
