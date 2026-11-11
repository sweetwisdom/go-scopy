//go:build !windows

package windowhost

import (
	"context"
	"errors"
)

const Supported = false

type Host struct{}

func New(string) *Host { return &Host{} }
func (*Host) Attach(context.Context, int) error {
	return errors.New("当前平台不支持灵动岛控制窗")
}
func (*Host) Focus()       {}
func (*Host) Release()     {}
func (*Host) Dock(bool)    {}
func (*Host) RefreshDock() {}
func (*Host) BeginDrag()   {}
func (*Host) MoveDrag()    {}
func (*Host) EndDrag()     {}
