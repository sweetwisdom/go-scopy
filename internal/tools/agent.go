package tools

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	agentDeviceJar   = "/data/local/tmp/go-scrcpy-agent.jar"
	agentSocket      = "localabstract:go-scrcpy-agent"
	agentMaxBlock    = 16 << 20
	agentMaxResponse = 64 << 20
	// Preserve native icon sizes; the bundled Agent truncates ratios when shrinking.
	agentIconSize = 1024
)

// listAppsAgent asks the on-device Android PackageManager for the complete
// app metadata and rendered PNG icons in one round trip.
func (t Tools) listAppsAgent(ctx context.Context, serial string) ([]AppInfo, error) {
	apps, err := t.readAgentApps(ctx, serial)
	if err == nil {
		return apps, nil
	}
	if startErr := t.startAgent(ctx, serial); startErr != nil {
		return nil, errors.Join(err, startErr)
	}
	var last error = err
	for i := 0; i < 12; i++ {
		apps, last = t.readAgentApps(ctx, serial)
		if last == nil {
			return apps, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("Go-Scrcpy Agent 启动后读取应用失败：%w", last)
}

func (t Tools) readAgentApps(ctx context.Context, serial string) ([]AppInfo, error) {
	conn, port, err := t.forwardAgent(ctx, serial)
	if err != nil {
		return nil, err
	}
	defer func() {
		conn.Close()
		_, _ = t.Device(context.Background(), serial, "forward", "--remove", "tcp:"+port)
	}()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(90 * time.Second))
	}
	for _, n := range []int32{10, 3, agentIconSize} {
		if err := binary.Write(conn, binary.BigEndian, n); err != nil {
			return nil, err
		}
	}
	return readAgentAppsResponse(conn)
}

func readAgentAppsResponse(conn io.Reader) ([]AppInfo, error) {
	// Per-block bounds alone still permit thousands of large retained icons.
	// Cap the complete response before allocating/retaining every app's data.
	conn = io.LimitReader(conn, agentMaxResponse)
	defaultIcon, err := readAgentBlock(conn)
	if err != nil {
		return nil, fmt.Errorf("读取 Go-Scrcpy Agent 默认图标：%w", err)
	}
	var count int32
	if err := binary.Read(conn, binary.BigEndian, &count); err != nil {
		return nil, err
	}
	if count < 0 || count > 10000 {
		return nil, fmt.Errorf("Go-Scrcpy Agent 返回了无效应用数量：%d", count)
	}
	apps := make([]AppInfo, 0, count)
	for i := int32(0); i < count; i++ {
		pkg, err := readAgentString(conn)
		if err != nil {
			return nil, fmt.Errorf("读取第 %d 个应用包名：%w", i, err)
		}
		version, err := readAgentString(conn)
		if err != nil {
			return nil, err
		}
		var versionCode int64
		if err = binary.Read(conn, binary.BigEndian, &versionCode); err != nil {
			return nil, err
		}
		label, err := readAgentString(conn)
		if err != nil {
			return nil, err
		}
		// install/update/last-used timestamps
		for n := 0; n < 3; n++ {
			var ignored int32
			if err = binary.Read(conn, binary.BigEndian, &ignored); err != nil {
				return nil, err
			}
		}
		// installer and CPU architecture
		for n := 0; n < 2; n++ {
			if _, err = readAgentString(conn); err != nil {
				return nil, err
			}
		}
		// target SDK, min SDK, and ApplicationInfo flags
		var targetSDK, minSDK, flags int32
		for _, value := range []*int32{&targetSDK, &minSDK, &flags} {
			if err = binary.Read(conn, binary.BigEndian, value); err != nil {
				return nil, err
			}
		}
		_, err = readAgentBool(conn) // has splits
		if err != nil {
			return nil, err
		}
		canLaunch, err := readAgentBool(conn)
		if err != nil {
			return nil, err
		}
		var appSize, dataSize, cacheSize int64
		for _, value := range []*int64{&appSize, &dataSize, &cacheSize} {
			if err = binary.Read(conn, binary.BigEndian, value); err != nil {
				return nil, err
			}
		}
		icon, err := readAgentBlock(conn)
		if err != nil {
			return nil, fmt.Errorf("读取 %s 图标：%w", pkg, err)
		}
		if len(icon) == 0 {
			icon = defaultIcon // Display fallback, not an application-specific icon.
		}
		if pkg == "" {
			continue
		}
		apps = append(apps, AppInfo{Package: pkg, Label: label, IconPNG: icon, VersionName: version, VersionCode: strconv.FormatInt(versionCode, 10), SizeBytes: appSize, System: flags&1 != 0 || flags&128 != 0, CanLaunch: canLaunch})
	}
	return apps, nil
}

func (t Tools) forwardAgent(ctx context.Context, serial string) (net.Conn, string, error) {
	out, err := t.Device(ctx, serial, "forward", "tcp:0", agentSocket)
	if err != nil {
		return nil, "", err
	}
	port := strings.TrimSpace(string(out))
	if _, err = strconv.Atoi(port); err != nil {
		return nil, "", fmt.Errorf("adb forward 返回了无效端口 %q", port)
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", port), 2*time.Second)
	if err != nil {
		_, _ = t.Device(context.Background(), serial, "forward", "--remove", "tcp:"+port)
		return nil, "", err
	}
	return conn, port, nil
}

func (t Tools) startAgent(ctx context.Context, serial string) error {
	jar, err := t.agentJarPath()
	if err != nil {
		return err
	}
	if _, err = t.Device(ctx, serial, "push", jar, agentDeviceJar); err != nil {
		return fmt.Errorf("部署 Go-Scrcpy Agent：%w", err)
	}
	command := "CLASSPATH=" + agentDeviceJar + " app_process / io.github.goscrcpy.agent.Main daemon >/dev/null 2>&1 &"
	if _, err = t.Device(ctx, serial, "shell", command); err != nil {
		return fmt.Errorf("启动 Go-Scrcpy Agent：%w", err)
	}
	return nil
}

func (t Tools) agentJarPath() (string, error) {
	candidates := []string{}
	if t.Root != "" {
		candidates = append(candidates,
			filepath.Join(t.Root, "tools", "scrcpy", "go-scrcpy-agent.jar"),
			filepath.Join(t.Root, "..", "Resources", "tools", "scrcpy", "go-scrcpy-agent.jar"),
			filepath.Join(t.Root, "resources", "extra", "common", "go-scrcpy-agent.jar"))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "resources", "extra", "common", "go-scrcpy-agent.jar"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", errors.New("找不到 Go-Scrcpy Agent：resources/extra/common/go-scrcpy-agent.jar")
}

func readAgentBlock(r io.Reader) ([]byte, error) {
	var size int32
	if err := binary.Read(r, binary.BigEndian, &size); err != nil {
		return nil, err
	}
	if size < 0 || size > agentMaxBlock {
		return nil, fmt.Errorf("Go-Scrcpy Agent 数据块长度无效：%d", size)
	}
	data := make([]byte, size)
	_, err := io.ReadFull(r, data)
	return data, err
}

func readAgentString(r io.Reader) (string, error) {
	data, err := readAgentBlock(r)
	return string(data), err
}

func readAgentBool(r io.Reader) (bool, error) {
	var value int32
	err := binary.Read(r, binary.BigEndian, &value)
	return value != 0, err
}
