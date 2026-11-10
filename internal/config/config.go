package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-scopy/go-scopy/internal/domain"
)

func Default() domain.Settings {
	home, _ := os.UserHomeDir()
	return domain.Settings{Theme: "light", OutputDir: filepath.Join(home, "Videos", "Go-Scopy"), Defaults: domain.Options{MaxSize: 1920, FPS: 60, Bitrate: 8, Audio: true}, Devices: map[string]domain.Options{}}
}

func Directory() (string, error) {
	p, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(p, "Go-Scopy"), nil
}

func Load(dir string) (domain.Settings, error) {
	s := Default()
	b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return Default(), fmt.Errorf("配置文件无法读取：%w", err)
	}
	if err = Validate(s); err != nil {
		return Default(), err
	}
	if s.Devices == nil {
		s.Devices = map[string]domain.Options{}
	}
	return s, nil
}

func ValidateOptions(o domain.Options) error {
	if o.MaxSize < 0 || o.MaxSize > 8192 {
		return errors.New("分辨率上限应为 0–8192，0 表示原始分辨率")
	}
	if o.FPS < 1 || o.FPS > 240 {
		return errors.New("帧率应为 1–240")
	}
	if o.Bitrate < 1 || o.Bitrate > 200 {
		return errors.New("码率应为 1–200 Mbps")
	}
	return nil
}

func Validate(s domain.Settings) error {
	if s.Theme != "light" && s.Theme != "dark" {
		return errors.New("外观必须是 light 或 dark")
	}
	if !filepath.IsAbs(s.OutputDir) {
		return errors.New("保存目录必须是绝对路径")
	}
	if err := ValidateOptions(s.Defaults); err != nil {
		return err
	}
	for _, o := range s.Devices {
		if err := ValidateOptions(o); err != nil {
			return err
		}
	}
	for serial, name := range s.DeviceNames {
		if strings.TrimSpace(serial) == "" || utf8.RuneCountInString(name) > 40 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			return errors.New("设备备注最多 40 个字符，且不能包含换行或控制字符")
		}
	}
	for address := range s.SavedWireless {
		if err := ValidateDeviceAddress(address); err != nil {
			return err
		}
	}
	return nil
}

func ValidateDeviceAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || strings.ContainsAny(host, " \t\n\r") {
		return errors.New("请输入有效的设备 IP:端口，例如 192.168.1.20:5555")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return errors.New("端口应为 1–65535")
	}
	return nil
}

// Save replaces the previous file only after a complete write and sync.
func Save(dir string, s domain.Settings) error {
	if err := Validate(s); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "settings-*.tmp")
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
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(dir, "settings.json"))
}
