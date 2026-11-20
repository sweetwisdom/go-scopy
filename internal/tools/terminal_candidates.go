package tools

import (
	"context"
	"errors"
	"slices"
	"strings"
)

// TerminalCandidates runs a fixed, read-only command. No shell text from the
// browser is interpolated into an executable or an Android shell command.
func (t Tools) TerminalCandidates(ctx context.Context, serial, kind string) ([]string, error) {
	var args []string
	switch kind {
	case "packages":
		args = []string{"pm", "list", "packages"}
	case "properties":
		args = []string{"getprop"}
	case "services":
		args = []string{"dumpsys", "-l"}
	default:
		return nil, errors.New("不支持的补全类型")
	}
	data, err := t.Device(ctx, serial, append([]string{"shell"}, args...)...)
	if err != nil {
		return nil, err
	}
	return parseTerminalCandidates(kind, string(data)), nil
}

func parseTerminalCandidates(kind, text string) []string {
	seen := map[string]bool{}
	var result []string
	for _, line := range strings.Split(text, "\n") {
		value := strings.TrimSpace(line)
		switch kind {
		case "packages":
			if !strings.HasPrefix(value, "package:") {
				continue
			}
			value = strings.TrimPrefix(value, "package:")
		case "properties":
			if !strings.HasPrefix(value, "[") {
				continue
			}
			end := strings.Index(value, "]:")
			if end < 1 {
				continue
			}
			value = value[1:end]
		case "services":
			if value == "" || strings.ContainsAny(value, " :\t") {
				continue
			}
		}
		if value == "" || len(value) > 256 || strings.ContainsAny(value, "\r\n\t ;|&$`\"'()<>\\") || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
		if len(result) >= 4096 {
			break
		}
	}
	slices.Sort(result)
	return result
}
