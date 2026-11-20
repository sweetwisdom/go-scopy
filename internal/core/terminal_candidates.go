package core

import "context"

func (s *Service) TerminalCandidates(ctx context.Context, serial, kind string) ([]string, error) {
	if err := s.online(serial); err != nil {
		return nil, err
	}
	return s.tool.TerminalCandidates(ctx, serial, kind)
}
