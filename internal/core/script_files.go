package core

import (
	"errors"

	"github.com/go-scopy/go-scopy/internal/scriptstore"
	"github.com/go-scopy/go-scopy/internal/scriptview"
)

func (s *Service) scriptFiles() (*scriptstore.Store, error) {
	if _, err := s.scriptManager(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("应用已关闭")
	}
	if s.scripts.files == nil {
		files, err := scriptstore.New(s.dir)
		if err != nil {
			return nil, err
		}
		s.scripts.files = files
	}
	return s.scripts.files, nil
}
func (s *Service) ScriptFiles() (scriptstore.Listing, error) {
	f, e := s.scriptFiles()
	if e != nil {
		return scriptstore.Listing{}, e
	}
	return f.List()
}
func (s *Service) OpenScriptFile(path string) (scriptstore.Document, error) {
	f, e := s.scriptFiles()
	if e != nil {
		return scriptstore.Document{}, e
	}
	return f.Open(path)
}
func (s *Service) ReadScriptFile(path string) (scriptstore.Document, error) {
	f, e := s.scriptFiles()
	if e != nil {
		return scriptstore.Document{}, e
	}
	return f.Read(path)
}
func (s *Service) NewScriptFile(name, content string) (scriptstore.Document, error) {
	f, e := s.scriptFiles()
	if e != nil {
		return scriptstore.Document{}, e
	}
	return f.Create(name, content)
}
func (s *Service) SaveScriptFile(path, content, version string) (scriptstore.Document, error) {
	f, e := s.scriptFiles()
	if e != nil {
		return scriptstore.Document{}, e
	}
	return f.Save(path, content, version)
}

var _ scriptview.FileBackend = (*Service)(nil)

func (s *Service) ScriptGuide() (string, error) {
	f, e := s.scriptFiles()
	if e != nil {
		return "", e
	}
	return f.Guide(scriptview.Guide())
}
