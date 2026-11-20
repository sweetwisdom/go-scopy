package domain

import "time"

// ScriptBackend is optional so existing workspace backends remain compatible.
// Scripts run with the current user's privileges; this is not a sandbox.
type ScriptBackend interface {
	ScriptState() ScriptState
	RunScript(ScriptRequest) (string, error)
	StopScript(id string)
}

type ScriptRequest struct {
	Path           string   `json:"path"`
	Serial         string   `json:"serial"`
	Python         string   `json:"python,omitempty"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
	Args           []string `json:"args,omitempty"`
}

type ScriptRun struct {
	ID        string        `json:"id"`
	Request   ScriptRequest `json:"request"`
	Status    string        `json:"status"`
	Started   time.Time     `json:"started"`
	Finished  time.Time     `json:"finished,omitempty"`
	ExitCode  int           `json:"exitCode"`
	Error     string        `json:"error,omitempty"`
	LogPath   string        `json:"logPath"`
	OutputDir string        `json:"outputDir"`
	Log       string        `json:"-"`
}

func (r ScriptRun) Active() bool { return r.Status == "running" || r.Status == "stopping" }

type ScriptState struct {
	Runs  []ScriptRun
	Last  ScriptRequest
	Error string
}
