package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	execsvc "github.com/samiralibabic/rexd/internal/exec"
	"github.com/samiralibabic/rexd/internal/audit"
	"github.com/samiralibabic/rexd/internal/config"
	"github.com/samiralibabic/rexd/internal/events"
	fssvc "github.com/samiralibabic/rexd/internal/fs"
	"github.com/samiralibabic/rexd/internal/policy"
	"github.com/samiralibabic/rexd/internal/protocol"
	"github.com/samiralibabic/rexd/internal/session"
)

const ServerVersion = "0.1.0"

type Service struct {
	cfg      config.Config
	sessions *session.Manager
	policy   *policy.Engine
	exec     *execsvc.Manager
	pty      *execsvc.PTYManager
	fs       *fssvc.Service
	bus      *events.Bus
	audit    *audit.Logger
}

func NewService(cfg config.Config) (*Service, error) {
	roots := config.AllowedRoots(cfg)
	pol, err := policy.New(roots, cfg.Security.AllowShell)
	if err != nil {
		return nil, err
	}
	bus := events.NewBus()
	return &Service{
		cfg:      cfg,
		sessions: session.NewManager(cfg.Limits.MaxConcurrentSessions),
		policy:   pol,
		exec:     execsvc.NewManager(bus),
		pty:      execsvc.NewPTYManager(bus),
		fs:       fssvc.NewService(int64(cfg.Limits.MaxFileReadBytes)),
		bus:      bus,
		audit:    audit.New(cfg.Audit.Enabled, cfg.Audit.Path),
	}, nil
}

func (s *Service) Bus() *events.Bus {
	return s.bus
}

func parseID(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var id any
	_ = json.Unmarshal(raw, &id)
	return id
}

func (s *Service) Handle(ctx context.Context, req protocol.Request) protocol.Response {
	id := parseID(req.ID)
	if req.JSONRPC != protocol.Version {
		return protocol.ErrorResponse(id, protocol.ErrInvalidParams, "jsonrpc must be 2.0", nil)
	}
	resp := protocol.Response{JSONRPC: protocol.Version, ID: id}
	switch req.Method {
	case "session.open":
		out, err := s.sessionOpen(req.Params)
		if err != nil {
			return s.errResp(id, err)
		}
		resp.Result = out
	case "session.info":
		out, err := s.sessionInfo(req.Params)
		if err != nil {
			return s.errResp(id, err)
		}
		resp.Result = out
	case "session.close":
		out, err := s.sessionClose(req.Params)
		if err != nil {
			return s.errResp(id, err)
		}
		resp.Result = out
	case "exec.start":
		out, err := s.execStart(ctx, req.Params)
		if err != nil {
			return s.errResp(id, err)
		}
		resp.Result = out
	case "exec.wait":
		out, err := s.execWait(ctx, req.Params)
		if err != nil {
			return s.errResp(id, err)
		}
		resp.Result = out
	case "exec.kill":
		out, err := s.execKill(req.Params)
		if err != nil {
			return s.errResp(id, err)
		}
		resp.Result = out
	case "exec.input":
		out, err := s.execInput(req.Params)
		if err != nil {
			return s.errResp(id, err)
		}
		resp.Result = out
	case "fs.read":
		out, err := s.fsRead(req.Params)
		if err != nil {
			return s.errResp(id, err)
		}
		resp.Result = out
	case "fs.write":
		out, err := s.fsWrite(req.Params)
		if err != nil {
			return s.errResp(id, err)
		}
		resp.Result = out
	case "fs.list":
		out, err := s.fsList(req.Params)
		if err != nil {
			return s.errResp(id, err)
		}
		resp.Result = out
	case "fs.glob":
		out, err := s.fsGlob(req.Params)
		if err != nil {
			return s.errResp(id, err)
		}
		resp.Result = out
	case "fs.stat":
		out, err := s.fsStat(req.Params)
		if err != nil {
			return s.errResp(id, err)
		}
		resp.Result = out
	case "pty.open":
		out, err := s.ptyOpen(ctx, req.Params)
		if err != nil {
			return s.errResp(id, err)
		}
		resp.Result = out
	case "pty.input":
		out, err := s.ptyInput(req.Params)
		if err != nil {
			return s.errResp(id, err)
		}
		resp.Result = out
	case "pty.resize":
		out, err := s.ptyResize(req.Params)
		if err != nil {
			return s.errResp(id, err)
		}
		resp.Result = out
	case "pty.close":
		out, err := s.ptyClose(req.Params)
		if err != nil {
			return s.errResp(id, err)
		}
		resp.Result = out
	default:
		return protocol.ErrorResponse(id, protocol.ErrMethodNotFound, "method not found", map[string]any{"method": req.Method})
	}
	s.audit.Write(audit.Entry{Method: req.Method, Result: resp.Result})
	return resp
}

func (s *Service) errResp(id any, err error) protocol.Response {
	switch {
	case errors.Is(err, session.ErrNotFound):
		return protocol.ErrorResponse(id, protocol.ErrInvalidParams, err.Error(), nil)
	case errors.Is(err, policy.ErrForbiddenPath):
		return protocol.ErrorResponse(id, protocol.ErrForbiddenPath, "Path is outside allowed roots", nil)
	case errors.Is(err, fssvc.ErrConflict):
		return protocol.ErrorResponse(id, protocol.ErrConcurrencyConflict, err.Error(), nil)
	case strings.Contains(err.Error(), "process not found"):
		return protocol.ErrorResponse(id, protocol.ErrProcessNotFound, err.Error(), nil)
	default:
		return protocol.ErrorResponse(id, protocol.ErrInvalidParams, err.Error(), nil)
	}
}

func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, err
	}
	return v, nil
}

func (s *Service) sessionOpen(raw json.RawMessage) (any, error) {
	p, err := decode[protocol.SessionOpenParams](raw)
	if err != nil {
		return nil, err
	}
	roots := s.policy.AllowedRoots()
	if len(p.WorkspaceRoots) > 0 {
		filtered := []string{}
		for _, requested := range p.WorkspaceRoots {
			for _, allowed := range roots {
				if requested == allowed {
					filtered = append(filtered, requested)
					break
				}
			}
		}
		if len(filtered) > 0 {
			roots = filtered
		}
	}
	sess, err := s.sessions.Open(p.ClientName, p.ClientVersion, roots)
	if err != nil {
		return nil, err
	}
	return protocol.SessionOpenResult{
		SessionID:      sess.ID,
		Protocol:       "rexd/1",
		ServerVersion:  ServerVersion,
		Capabilities:   []string{"exec", "fs", "events", "pty", "http"},
		Limits:         map[string]int{"default_timeout_ms": s.cfg.Limits.DefaultTimeoutMs, "max_output_bytes": s.cfg.Limits.MaxOutputBytes},
		WorkspaceRoots: roots,
	}, nil
}

func (s *Service) sessionInfo(raw json.RawMessage) (any, error) {
	p, err := decode[protocol.SessionInfoParams](raw)
	if err != nil {
		return nil, err
	}
	sess, err := s.sessions.Get(p.SessionID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"session_id":       sess.ID,
		"cwd":              sess.CWD,
		"workspace_roots":  sess.WorkspaceRoots,
		"running_processes": sess.ProcessCount,
		"limits": map[string]any{
			"default_timeout_ms": s.cfg.Limits.DefaultTimeoutMs,
			"hard_timeout_ms":    s.cfg.Limits.HardTimeoutMs,
			"max_output_bytes":   s.cfg.Limits.MaxOutputBytes,
		},
	}, nil
}

func (s *Service) sessionClose(raw json.RawMessage) (any, error) {
	p, err := decode[protocol.SessionCloseParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.sessions.Close(p.SessionID); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func buildEnv(extra map[string]string) []string {
	env := []string{}
	for k, v := range extra {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

func (s *Service) execStart(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decode[protocol.ExecStartParams](raw)
	if err != nil {
		return nil, err
	}
	sess, err := s.sessions.Get(p.SessionID)
	if err != nil {
		return nil, err
	}
	if sess.ProcessCount >= s.cfg.Limits.MaxProcessesPerSess {
		return nil, errors.New("max processes per session reached")
	}
	cwd := sess.CWD
	if p.Cwd != "" {
		cwd, err = s.policy.ResolvePath(sess.CWD, p.Cwd)
		if err != nil {
			return nil, err
		}
	}
	var cmd *exec.Cmd
	if p.Shell {
		if !s.policy.AllowShell() {
			return nil, errors.New("shell mode disabled")
		}
		if p.Command == "" {
			return nil, errors.New("command required when shell=true")
		}
		cmd = exec.CommandContext(ctx, "sh", "-lc", p.Command)
	} else {
		if len(p.Argv) == 0 {
			return nil, errors.New("argv is required")
		}
		cmd = exec.CommandContext(ctx, p.Argv[0], p.Argv[1:]...)
	}
	cmd.Dir = cwd
	if len(p.Env) > 0 {
		cmd.Env = append(cmd.Env, buildEnv(p.Env)...)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	if err := s.sessions.IncProcess(sess.ID); err != nil {
		return nil, err
	}
	maxOutput := p.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = s.cfg.Limits.MaxOutputBytes
	}
	timeoutMS := p.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = s.cfg.Limits.DefaultTimeoutMs
	}
	if timeoutMS > s.cfg.Limits.HardTimeoutMs {
		timeoutMS = s.cfg.Limits.HardTimeoutMs
	}
	processID := "p_" + fmt.Sprintf("%d", time.Now().UnixNano())
	execCtx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMS)*time.Millisecond)
	rp := &execsvc.RunningProcess{
		ID:            processID,
		SessionID:     sess.ID,
		Cmd:           cmd,
		Stdin:         stdin,
		StartedAt:     time.Now().UTC(),
		MaxOutput:     int64(maxOutput),
		Detached:      p.Detach,
		ExitCh:        make(chan execsvc.ProcessState, 1),
	}
	rp.CancelTimeout(cancel)
	s.exec.Add(rp)
	s.exec.WireStreams(rp, stdout, stderr)
	if p.Stdin != "" {
		_, _ = stdin.Write([]byte(p.Stdin))
		_ = stdin.Close()
	}
	go func() {
		<-execCtx.Done()
		rp.MarkTimedOut()
		_ = cmd.Process.Kill()
	}()
	go func() {
		waitErr := cmd.Wait()
		state := s.exec.Wait(rp, waitErr)
		rp.ExitCh <- state
		s.bus.Publish(sess.ID, "exec.exit", map[string]any{
			"session_id":   sess.ID,
			"process_id":   rp.ID,
			"exit_code":    state.ExitCode,
			"signal":       state.Signal,
			"timed_out":    state.TimedOut,
			"duration_ms":  int(time.Since(rp.StartedAt).Milliseconds()),
			"bytes_stdout": state.BytesStdout,
			"bytes_stderr": state.BytesStderr,
		})
		s.exec.Remove(rp.ID)
		_ = s.sessions.DecProcess(sess.ID)
	}()
	return protocol.ExecStartResult{
		ProcessID: processID,
		StartedAt: rp.StartedAt.Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) execWait(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decode[protocol.ExecWaitParams](raw)
	if err != nil {
		return nil, err
	}
	rp, err := s.exec.Get(p.ProcessID)
	if err != nil {
		return nil, err
	}
	timeout := p.TimeoutMS
	if timeout <= 0 {
		timeout = s.cfg.Limits.DefaultTimeoutMs
	}
	select {
	case st := <-rp.ExitCh:
		return protocol.ExecWaitResult{
			Status:      st.Status,
			ExitCode:    st.ExitCode,
			Signal:      st.Signal,
			BytesStdout: st.BytesStdout,
			BytesStderr: st.BytesStderr,
		}, nil
	case <-time.After(time.Duration(timeout) * time.Millisecond):
		return protocol.ExecWaitResult{Status: "running"}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Service) execKill(raw json.RawMessage) (any, error) {
	p, err := decode[protocol.ExecKillParams](raw)
	if err != nil {
		return nil, err
	}
	sig := p.Signal
	if sig == "" {
		sig = "TERM"
	}
	if err := s.exec.Kill(p.ProcessID, sig); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func (s *Service) execInput(raw json.RawMessage) (any, error) {
	p, err := decode[protocol.ExecInputParams](raw)
	if err != nil {
		return nil, err
	}
	rp, err := s.exec.Get(p.ProcessID)
	if err != nil {
		return nil, err
	}
	n, err := rp.Stdin.Write([]byte(p.Data))
	if err != nil {
		return nil, err
	}
	if p.EOF {
		_ = rp.Stdin.Close()
	}
	return protocol.ExecInputResult{AcceptedBytes: n}, nil
}

func (s *Service) resolveSessionPath(sessionID, inputPath string) (string, error) {
	sess, err := s.sessions.Get(sessionID)
	if err != nil {
		return "", err
	}
	return s.policy.ResolvePath(sess.CWD, inputPath)
}

func (s *Service) fsRead(raw json.RawMessage) (any, error) {
	p, err := decode[protocol.FSReadParams](raw)
	if err != nil {
		return nil, err
	}
	abs, err := s.resolveSessionPath(p.SessionID, p.Path)
	if err != nil {
		return nil, err
	}
	return s.fs.Read(abs, p.Encoding, p.Offset, p.Length)
}

func (s *Service) fsWrite(raw json.RawMessage) (any, error) {
	p, err := decode[protocol.FSWriteParams](raw)
	if err != nil {
		return nil, err
	}
	abs, err := s.resolveSessionPath(p.SessionID, p.Path)
	if err != nil {
		return nil, err
	}
	content, err := fssvc.DecodeContent(p.Content, p.Encoding)
	if err != nil {
		return nil, err
	}
	mode := p.Mode
	if mode == "" {
		mode = "replace"
	}
	return s.fs.Write(abs, content, mode, p.MkdirParents, true, p.ExpectedMTime)
}

func (s *Service) fsList(raw json.RawMessage) (any, error) {
	p, err := decode[protocol.FSListParams](raw)
	if err != nil {
		return nil, err
	}
	abs, err := s.resolveSessionPath(p.SessionID, p.Path)
	if err != nil {
		return nil, err
	}
	return s.fs.List(abs, p.Recursive, p.MaxEntries)
}

func (s *Service) fsGlob(raw json.RawMessage) (any, error) {
	p, err := decode[protocol.FSGlobParams](raw)
	if err != nil {
		return nil, err
	}
	sess, err := s.sessions.Get(p.SessionID)
	if err != nil {
		return nil, err
	}
	cwd := sess.CWD
	if p.Cwd != "" {
		cwd, err = s.policy.ResolvePath(sess.CWD, p.Cwd)
		if err != nil {
			return nil, err
		}
	}
	pattern := p.Pattern
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(cwd, pattern)
	}
	matches, err := s.fs.Glob(pattern, p.MaxMatches)
	if err != nil {
		return nil, err
	}
	filtered := make([]string, 0, len(matches))
	for _, m := range matches {
		if s.policy.IsAllowed(m) {
			filtered = append(filtered, m)
		}
	}
	return map[string]any{"matches": filtered}, nil
}

func (s *Service) fsStat(raw json.RawMessage) (any, error) {
	p, err := decode[protocol.FSStatParams](raw)
	if err != nil {
		return nil, err
	}
	abs, err := s.resolveSessionPath(p.SessionID, p.Path)
	if err != nil {
		return nil, err
	}
	return s.fs.Stat(abs)
}

func (s *Service) ptyOpen(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decode[protocol.PTYOpenParams](raw)
	if err != nil {
		return nil, err
	}
	sess, err := s.sessions.Get(p.SessionID)
	if err != nil {
		return nil, err
	}
	cwd := sess.CWD
	if p.Cwd != "" {
		cwd, err = s.policy.ResolvePath(sess.CWD, p.Cwd)
		if err != nil {
			return nil, err
		}
	}
	if p.Shell && !s.policy.AllowShell() {
		return nil, errors.New("shell mode disabled")
	}
	ptySession, err := s.pty.Open(ctx, p.SessionID, p.Argv, p.Shell, p.Command, cwd, p.Env, p.Cols, p.Rows)
	if err != nil {
		return nil, err
	}
	return protocol.PTYOpenResult{
		PTYID:     ptySession.ID,
		ProcessID: ptySession.ProcessID,
	}, nil
}

func (s *Service) ptyInput(raw json.RawMessage) (any, error) {
	p, err := decode[protocol.PTYInputParams](raw)
	if err != nil {
		return nil, err
	}
	n, err := s.pty.Input(p.PTYID, p.Data)
	if err != nil {
		return nil, err
	}
	return map[string]any{"accepted_bytes": n}, nil
}

func (s *Service) ptyResize(raw json.RawMessage) (any, error) {
	p, err := decode[protocol.PTYResizeParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.pty.Resize(p.PTYID, p.Cols, p.Rows); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func (s *Service) ptyClose(raw json.RawMessage) (any, error) {
	p, err := decode[protocol.PTYCloseParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.pty.Close(p.PTYID); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}
