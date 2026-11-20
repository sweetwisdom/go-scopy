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
	Dependencies *ScriptDependencies
	Runs         []ScriptRun
	Last         ScriptRequest
	Error        string
}

// ScriptDependencyBackend is optional; installation only starts on an explicit
// user action and always targets the same interpreter used by the runner.
type ScriptDependencyBackend interface {
	CheckScriptDependencies(python string) error
	InstallScriptDependencies(python string) error
	StopScriptDependencies()
}

type ScriptDependencies struct {
	RequestedPython string `json:"requestedPython"`
	Python          string `json:"python"`
	Operation       string `json:"operation"`
	Status          string `json:"status"`
	Ready           bool   `json:"ready"`
	Error           string `json:"error"`
	Log             string `json:"log"`
}
