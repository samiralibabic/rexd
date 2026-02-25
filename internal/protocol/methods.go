package protocol

type SessionOpenParams struct {
	ClientName            string   `json:"client_name"`
	ClientVersion         string   `json:"client_version,omitempty"`
	WorkspaceRoots        []string `json:"workspace_roots,omitempty"`
	RequestedCapabilities []string `json:"requested_capabilities,omitempty"`
}

type SessionOpenResult struct {
	SessionID      string         `json:"session_id"`
	Protocol       string         `json:"protocol"`
	ServerVersion  string         `json:"server_version"`
	Capabilities   []string       `json:"capabilities"`
	Limits         map[string]int `json:"limits"`
	WorkspaceRoots []string       `json:"workspace_roots"`
}

type SessionCloseParams struct {
	SessionID string `json:"session_id"`
}

type SessionInfoParams struct {
	SessionID string `json:"session_id"`
}

type ExecStartParams struct {
	SessionID      string            `json:"session_id"`
	Argv           []string          `json:"argv,omitempty"`
	Cwd            string            `json:"cwd,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Stdin          string            `json:"stdin,omitempty"`
	TimeoutMS      int               `json:"timeout_ms,omitempty"`
	MaxOutputBytes int               `json:"max_output_bytes,omitempty"`
	Shell          bool              `json:"shell,omitempty"`
	Command        string            `json:"command,omitempty"`
	Detach         bool              `json:"detach,omitempty"`
}

type ExecStartResult struct {
	ProcessID string `json:"process_id"`
	StartedAt string `json:"started_at"`
}

type ExecWaitParams struct {
	SessionID string `json:"session_id"`
	ProcessID string `json:"process_id"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
}

type ExecWaitResult struct {
	Status      string  `json:"status"`
	ExitCode    *int    `json:"exit_code"`
	Signal      *string `json:"signal"`
	BytesStdout int64   `json:"bytes_stdout"`
	BytesStderr int64   `json:"bytes_stderr"`
}

type ExecKillParams struct {
	SessionID string `json:"session_id"`
	ProcessID string `json:"process_id"`
	Signal    string `json:"signal,omitempty"`
}

type ExecInputParams struct {
	SessionID string `json:"session_id"`
	ProcessID string `json:"process_id"`
	Data      string `json:"data"`
	EOF       bool   `json:"eof,omitempty"`
}

type ExecInputResult struct {
	AcceptedBytes int `json:"accepted_bytes"`
}

type FSReadParams struct {
	SessionID string `json:"session_id"`
	Path      string `json:"path"`
	Offset    int64  `json:"offset,omitempty"`
	Length    int64  `json:"length,omitempty"`
	Encoding  string `json:"encoding,omitempty"`
}

type FSWriteParams struct {
	SessionID     string `json:"session_id"`
	Path          string `json:"path"`
	Content       string `json:"content"`
	Encoding      string `json:"encoding,omitempty"`
	Mode          string `json:"mode,omitempty"`
	MkdirParents  bool   `json:"mkdir_parents,omitempty"`
	Atomic        bool   `json:"atomic,omitempty"`
	ExpectedMTime int64  `json:"expected_mtime,omitempty"`
}

type FSListParams struct {
	SessionID  string `json:"session_id"`
	Path       string `json:"path"`
	Recursive  bool   `json:"recursive,omitempty"`
	MaxEntries int    `json:"max_entries,omitempty"`
}

type FSGlobParams struct {
	SessionID  string `json:"session_id"`
	Pattern    string `json:"pattern"`
	Cwd        string `json:"cwd,omitempty"`
	MaxMatches int    `json:"max_matches,omitempty"`
}

type FSStatParams struct {
	SessionID string `json:"session_id"`
	Path      string `json:"path"`
}

type PTYOpenParams struct {
	SessionID string            `json:"session_id"`
	Argv      []string          `json:"argv,omitempty"`
	Command   string            `json:"command,omitempty"`
	Shell     bool              `json:"shell,omitempty"`
	Cwd       string            `json:"cwd,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Cols      uint16            `json:"cols,omitempty"`
	Rows      uint16            `json:"rows,omitempty"`
}

type PTYOpenResult struct {
	PTYID     string `json:"pty_id"`
	ProcessID string `json:"process_id"`
}

type PTYInputParams struct {
	SessionID string `json:"session_id"`
	PTYID     string `json:"pty_id"`
	Data      string `json:"data"`
}

type PTYResizeParams struct {
	SessionID string `json:"session_id"`
	PTYID     string `json:"pty_id"`
	Cols      uint16 `json:"cols"`
	Rows      uint16 `json:"rows"`
}

type PTYCloseParams struct {
	SessionID string `json:"session_id"`
	PTYID     string `json:"pty_id"`
}
