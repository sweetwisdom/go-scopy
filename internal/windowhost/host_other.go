//go:build !windows

package windowhost

import (
	"context"
	"errors"
)

const Supported = false

type Host struct{}

func New(string, int, bool) *Host { return &Host{} }
func (*Host) Attach(context.Context, int) error {
	return errors.New("当前平台不支持窗口嵌入")
}
func (*Host) Resize()           {}
func (*Host) Sync()             {}
func (*Host) Focus()            {}
func (*Host) Release()          {}
func (*Host) Dock(string, bool) {}
