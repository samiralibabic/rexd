package policy

import (
	"errors"
	"path/filepath"
	"strings"
)

var ErrForbiddenPath = errors.New("path is outside allowed roots")

type Engine struct {
	allowedRoots []string
	allowShell   bool
}

func New(allowedRoots []string, allowShell bool) (*Engine, error) {
	norm := make([]string, 0, len(allowedRoots))
	for _, root := range allowedRoots {
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		norm = append(norm, filepath.Clean(abs))
	}
	return &Engine{allowedRoots: norm, allowShell: allowShell}, nil
}

func (e *Engine) AllowedRoots() []string {
	cp := make([]string, len(e.allowedRoots))
	copy(cp, e.allowedRoots)
	return cp
}

func (e *Engine) AllowShell() bool {
	return e.allowShell
}

func (e *Engine) ResolvePath(cwd, p string) (string, error) {
	candidate := p
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(cwd, candidate)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	cleaned := filepath.Clean(abs)
	if !e.IsAllowed(cleaned) {
		return "", ErrForbiddenPath
	}
	return cleaned, nil
}

func (e *Engine) IsAllowed(path string) bool {
	if len(e.allowedRoots) == 0 {
		return false
	}
	cleaned := filepath.Clean(path)
	for _, root := range e.allowedRoots {
		if cleaned == root {
			return true
		}
		if strings.HasPrefix(cleaned, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
