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
	pandaDeviceJar = "/data/local/tmp/panda-kit.jar"
	pandaSocket    = "localabstract:panda-1.1.0"
	pandaMaxBlock  = 16 << 20
)

// listAppsPanda asks the on-device Android PackageManager for the complete
// app metadata and rendered PNG icons in one round trip.
func (t Tools) listAppsPanda(ctx context.Context, serial string) ([]AppInfo, error) {
	apps, err := t.readPandaApps(ctx, serial)
	if err == nil {
		return apps, nil
	}
	if startErr := t.startPanda(ctx, serial); startErr != nil {
		return nil, errors.Join(err, startErr)
	}
	var last error = err
	for i := 0; i < 12; i++ {
		apps, last = t.readPandaApps(ctx, serial)
		if last == nil {
			return apps, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("PandaKit 启动后读取应用失败：%w", last)
}

func (t Tools) readPandaApps(ctx context.Context, serial string) ([]AppInfo, error) {
	conn, port, err := t.forwardPanda(ctx, serial)
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
	for _, n := range []int32{10, 3, 64} {
		if err := binary.Write(conn, binary.BigEndian, n); err != nil {
			return nil, err
		}
	}
	if _, err := readPandaBlock(conn); err != nil { // default icon
		return nil, fmt.Errorf("读取 PandaKit 默认图标：%w", err)
	}
	var count int32
	if err := binary.Read(conn, binary.BigEndian, &count); err != nil {
		return nil, err
	}
	if count < 0 || count > 10000 {
		return nil, fmt.Errorf("PandaKit 返回了无效应用数量：%d", count)
	}
	apps := make([]AppInfo, 0, count)
	for i := int32(0); i < count; i++ {
		pkg, err := readPandaString(conn)
		if err != nil {
			return nil, fmt.Errorf("读取第 %d 个应用包名：%w", i, err)
		}
		version, err := readPandaString(conn)
		if err != nil {
			return nil, err
		}
		var versionCode int64
		if err = binary.Read(conn, binary.BigEndian, &versionCode); err != nil {
			return nil, err
		}
		label, err := readPandaString(conn)
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
			if _, err = readPandaString(conn); err != nil {
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
		_, err = readPandaBool(conn) // has splits
		if err != nil {
			return nil, err
		}
		canLaunch, err := readPandaBool(conn)
		if err != nil {
			return nil, err
		}
		var appSize, dataSize, cacheSize int64
		for _, value := range []*int64{&appSize, &dataSize, &cacheSize} {
			if err = binary.Read(conn, binary.BigEndian, value); err != nil {
				return nil, err
			}
		}
		icon, err := readPandaBlock(conn)
		if err != nil {
			return nil, fmt.Errorf("读取 %s 图标：%w", pkg, err)
		}
		if pkg == "" {
			continue
		}
		apps = append(apps, AppInfo{Package: pkg, Label: label, IconPNG: icon, VersionName: version, VersionCode: strconv.FormatInt(versionCode, 10), SizeBytes: appSize, System: flags&1 != 0 || flags&128 != 0, CanLaunch: canLaunch})
	}
	return apps, nil
}

func (t Tools) forwardPanda(ctx context.Context, serial string) (net.Conn, string, error) {
	out, err := t.Device(ctx, serial, "forward", "tcp:0", pandaSocket)
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

func (t Tools) startPanda(ctx context.Context, serial string) error {
	jar, err := t.pandaJarPath()
	if err != nil {
		return err
	}
	if _, err = t.Device(ctx, serial, "push", jar, pandaDeviceJar); err != nil {
		return fmt.Errorf("部署 PandaKit：%w", err)
	}
	command := "CLASSPATH=" + pandaDeviceJar + " app_process / com.panda.Main >/dev/null 2>&1 &"
	if _, err = t.Device(ctx, serial, "shell", command); err != nil {
		return fmt.Errorf("启动 PandaKit：%w", err)
	}
	return nil
}

func (t Tools) pandaJarPath() (string, error) {
	candidates := []string{}
	if t.Root != "" {
		candidates = append(candidates,
			filepath.Join(t.Root, "tools", "scrcpy", "panda-kit.jar"),
			filepath.Join(t.Root, "..", "Resources", "tools", "scrcpy", "panda-kit.jar"),
			filepath.Join(t.Root, "resources", "extra", "common", "panda-kit.jar"))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "resources", "extra", "common", "panda-kit.jar"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", errors.New("找不到 PandaKit：resources/extra/common/panda-kit.jar")
}

func readPandaBlock(r io.Reader) ([]byte, error) {
	var size int32
	if err := binary.Read(r, binary.BigEndian, &size); err != nil {
		return nil, err
	}
	if size < 0 || size > pandaMaxBlock {
		return nil, fmt.Errorf("PandaKit 数据块长度无效：%d", size)
	}
	data := make([]byte, size)
	_, err := io.ReadFull(r, data)
	return data, err
}

func readPandaString(r io.Reader) (string, error) {
	data, err := readPandaBlock(r)
	return string(data), err
}

func readPandaBool(r io.Reader) (bool, error) {
	var value int32
	err := binary.Read(r, binary.BigEndian, &value)
	return value != 0, err
}
