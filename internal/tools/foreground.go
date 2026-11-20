package tools

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	activityDisplay   = regexp.MustCompile(`(?m)^\s*Display #(\d+)\b`)
	activityComponent = regexp.MustCompile(`\b([a-zA-Z][a-zA-Z0-9_]*(?:\.[a-zA-Z0-9_]+)*)/[a-zA-Z0-9_.$]+`)
)

// resumedApps only accepts resumed activities within an explicit display
// section. Global focused/resumed records may belong to another virtual screen.
func resumedApps(text string) map[int]string {
	apps := map[int]string{}
	display, displayIndent := -1, -1
	top := map[int]bool{}
	for _, line := range strings.Split(text, "\n") {
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if m := activityDisplay.FindStringSubmatch(line); m != nil {
			display, _ = strconv.Atoi(m[1])
			displayIndent = indent
			continue
		}
		if strings.TrimSpace(line) != "" && indent <= displayIndent {
			display = -1
		}
		if display < 0 || !(strings.Contains(line, "mResumedActivity") || strings.Contains(line, "topResumedActivity")) {
			continue
		}
		if m := activityComponent.FindStringSubmatch(line); m != nil {
			isTop := strings.Contains(line, "topResumedActivity")
			if isTop || !top[display] {
				apps[display] = m[1]
				top[display] = isTop
			}
		}
	}
	return apps
}

// ForegroundApp resolves display 0, never whichever virtual display last gained
// focus. Fail closed if Android does not expose a recognizable resumed activity.
func (t Tools) ForegroundApp(ctx context.Context, serial string) (string, error) {
	return foregroundApp(ctx, func(ctx context.Context, args ...string) ([]byte, error) {
		return t.Device(ctx, serial, args...)
	})
}

type foregroundCommand func(context.Context, ...string) ([]byte, error)

func foregroundApp(ctx context.Context, run foregroundCommand) (string, error) {
	out, err := run(ctx, "shell", "dumpsys", "activity", "activities")
	if err != nil {
		return "", fmt.Errorf("读取前台应用失败：%w", err)
	}
	pkg := resumedApps(string(out))[0]
	if pkg == "" {
		return "", errors.New("未识别到主屏前台应用，请打开应用后重试")
	}
	if pkg == "com.android.systemui" || pkg == "android" {
		return "", errors.New("当前为系统界面，请先打开需要独立的应用")
	}
	out, err = run(ctx, "shell", "cmd", "package", "resolve-activity", "--brief", "--user", "current", "-a", "android.intent.action.MAIN", "-c", "android.intent.category.HOME")
	if err != nil {
		return "", fmt.Errorf("识别桌面应用失败：%w", err)
	}
	matches := activityComponent.FindAllStringSubmatch(string(out), -1)
	if len(matches) == 0 {
		return "", errors.New("未识别到桌面应用，已取消独立窗口操作")
	}
	if pkg == matches[len(matches)-1][1] {
		return "", errors.New("当前已在手机桌面，请先打开需要独立的应用")
	}
	return pkg, nil
}

// DisplayIDs snapshots existing displays before a shortcut starts scrcpy. An
// older virtual window running the same package must not satisfy readiness.
func (t Tools) DisplayIDs(ctx context.Context, serial string) (map[int]bool, error) {
	out, err := t.Device(ctx, serial, "shell", "dumpsys", "activity", "activities")
	if err != nil {
		return nil, fmt.Errorf("读取现有显示器失败：%w", err)
	}
	displays := map[int]bool{}
	for _, match := range activityDisplay.FindAllStringSubmatch(string(out), -1) {
		id, _ := strconv.Atoi(match[1])
		displays[id] = true
	}
	if !displays[0] {
		return nil, errors.New("未识别到主屏显示器，已取消独立窗口操作")
	}
	return displays, nil
}

// ReturnHomeAfterApp waits for the requested app to actually run on a secondary
// display, not merely for the scrcpy process/window to exist. HOME is explicitly
// targeted at display 0 so it cannot dismiss the app in its new window.
func (t Tools) ReturnHomeAfterApp(ctx context.Context, serial, pkg string, existing map[int]bool) error {
	return returnHomeAfterApp(ctx, pkg, existing, func(ctx context.Context, args ...string) ([]byte, error) {
		return t.Device(ctx, serial, args...)
	})
}

func returnHomeAfterApp(ctx context.Context, pkg string, existing map[int]bool, run foregroundCommand) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		out, err := run(ctx, "shell", "dumpsys", "activity", "activities")
		if err != nil {
			return fmt.Errorf("确认独立应用就绪失败：%w", err)
		}
		for display, app := range resumedApps(string(out)) {
			if display != 0 && !existing[display] && app == pkg {
				_, err = run(ctx, "shell", "input", "-d", "0", "keyevent", "3")
				if err != nil {
					return fmt.Errorf("独立应用已启动，但主屏返回桌面失败：%w", err)
				}
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("未确认应用在独立窗口运行，未发送返回桌面操作：%w", ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}
