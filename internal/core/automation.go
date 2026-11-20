package core

import (
	"errors"

	"github.com/go-scopy/go-scopy/internal/automation"
	"github.com/go-scopy/go-scopy/internal/domain"
	"github.com/go-scopy/go-scopy/internal/scriptstore"
)

type scriptService struct {
	manager *automation.Manager
	files   *scriptstore.Store
}

func (s *Service) scriptManager() (*automation.Manager, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("应用已关闭")
	}
	if s.scripts == nil {
		s.scripts = &scriptService{manager: automation.New(s.tool.Root, s.dir, s.tool.ADB, s.wake)}
	}
	return s.scripts.manager, nil
}
func (s *Service) ScriptState() domain.ScriptState {
	m, e := s.scriptManager()
	if e != nil {
		return domain.ScriptState{Error: e.Error()}
	}
	return m.Snapshot()
}
func (s *Service) RunScript(req domain.ScriptRequest) (string, error) {
	if err := s.online(req.Serial); err != nil {
		return "", err
	}
	m, e := s.scriptManager()
	if e != nil {
		return "", e
	}
	return m.Start(req)
}
func (s *Service) StopScript(id string) {
	m, e := s.scriptManager()
	if e == nil {
		m.Stop(id)
	}
}
func (s *Service) closeScripts() {
	s.mu.Lock()
	v := s.scripts
	s.mu.Unlock()
	if v != nil {
		v.manager.Close()
	}
}

var _ domain.ScriptBackend = (*Service)(nil)

func (s *Service) CheckScriptDependencies(python string) error {
	m, err := s.scriptManager()
	if err != nil {
		return err
	}
	return m.Dependencies(python, false)
}
func (s *Service) InstallScriptDependencies(python string) error {
	m, err := s.scriptManager()
	if err != nil {
		return err
	}
	return m.Dependencies(python, true)
}
func (s *Service) StopScriptDependencies() {
	m, err := s.scriptManager()
	if err == nil {
		m.StopDependencies()
	}
}

var _ domain.ScriptDependencyBackend = (*Service)(nil)
