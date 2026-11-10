package tools

import (
	"context"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-scopy/go-scopy/internal/domain"
)

func ParseDevices(text string) []domain.Device {
	var result []domain.Device
	for _, line := range strings.Split(text, "\n") {
		p := strings.Fields(line)
		if len(p) < 2 || p[0] == "List" || strings.HasPrefix(p[0], "*") {
			continue
		}
		d := domain.Device{Serial: p[0], State: p[1], Connection: "USB"}
		if strings.Contains(d.Serial, ":") || strings.Contains(d.Serial, "._adb-tls-connect.") {
			d.Connection = "Wi-Fi"
		}
		for _, field := range p[2:] {
			key, value, ok := strings.Cut(field, ":")
			if !ok {
				continue
			}
			switch key {
			case "model":
				d.Model = strings.ReplaceAll(value, "_", " ")
			case "product":
				d.Product = value
			}
		}
		result = append(result, d)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Serial < result[j].Serial })
	return result
}

func readFrame(r io.Reader) (string, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return "", err
	}
	n, err := strconv.ParseUint(string(header[:]), 16, 16)
	if err != nil {
		return "", fmt.Errorf("无效的 adb 数据长度：%q", header)
	}
	b := make([]byte, int(n))
	_, err = io.ReadFull(r, b)
	return string(b), err
}

// Track subscribes to the local adb smart socket; it sleeps in Read while idle.
// The adb CLI and scrcpy are explicitly pointed at this same server.
func Track(ctx context.Context, onDevices func([]domain.Device)) error {
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", "127.0.0.1:5037")
	if err != nil {
		return err
	}
	return trackConnection(ctx, conn, onDevices)
}

func trackConnection(ctx context.Context, conn net.Conn, onDevices func([]domain.Device)) error {
	defer conn.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()
	request := "host:track-devices-l"
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := fmt.Fprintf(conn, "%04x%s", len(request), request); err != nil {
		return err
	}
	var status [4]byte
	if _, err := io.ReadFull(conn, status[:]); err != nil {
		return err
	}
	if string(status[:]) != "OKAY" {
		message, _ := readFrame(conn)
		return fmt.Errorf("adb 订阅失败：%s %s", status, message)
	}
	conn.SetDeadline(time.Time{})
	for {
		text, err := readFrame(conn)
		if err != nil {
			return err
		}
		onDevices(ParseDevices(text))
	}
}
