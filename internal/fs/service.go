package fs

import (
	"encoding/base64"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

var ErrConflict = errors.New("expected mtime does not match")

type Service struct {
	maxReadBytes int64
}

func NewService(maxReadBytes int64) *Service {
	return &Service{maxReadBytes: maxReadBytes}
}

func (s *Service) Read(path, encoding string, offset, length int64) (map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil, err
		}
	}
	limit := s.maxReadBytes
	if length > 0 && (limit <= 0 || length < limit) {
		limit = length
	}
	if limit <= 0 {
		limit = st.Size()
	}
	buf := make([]byte, limit)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	buf = buf[:n]
	content := string(buf)
	if encoding == "base64" {
		content = base64.StdEncoding.EncodeToString(buf)
	} else {
		encoding = "utf8"
	}
	return map[string]any{
		"path":      path,
		"size":      st.Size(),
		"mtime":     st.ModTime().UnixMilli(),
		"encoding":  encoding,
		"content":   content,
		"truncated": int64(n) < st.Size()-offset,
	}, nil
}

func (s *Service) Write(path string, data []byte, mode string, mkdirParents bool, atomic bool, expectedMTime int64) (map[string]any, error) {
	if mkdirParents {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
	}
	existed := false
	if st, err := os.Stat(path); err == nil {
		existed = true
		if expectedMTime > 0 && st.ModTime().UnixMilli() != expectedMTime {
			return nil, ErrConflict
		}
	}
	flags := os.O_CREATE | os.O_WRONLY
	switch mode {
	case "append":
		flags |= os.O_APPEND
	case "create":
		flags |= os.O_EXCL
	default:
		flags |= os.O_TRUNC
	}
	if atomic && mode != "append" {
		tmp := path + ".rexd.tmp"
		if err := os.WriteFile(tmp, data, 0644); err != nil {
			return nil, err
		}
		if err := os.Rename(tmp, path); err != nil {
			return nil, err
		}
	} else {
		f, err := os.OpenFile(path, flags, 0644)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(data); err != nil {
			_ = f.Close()
			return nil, err
		}
		_ = f.Close()
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"path":          path,
		"bytes_written": len(data),
		"mtime":         st.ModTime().UnixMilli(),
		"created":       !existed,
	}, nil
}

func (s *Service) List(path string, recursive bool, maxEntries int) (map[string]any, error) {
	entries := []map[string]any{}
	appendEntry := func(fullPath string, d fs.DirEntry, info fs.FileInfo) {
		var mt any
		if info != nil {
			mt = info.ModTime().UnixMilli()
		}
		var sz any
		if info != nil && !d.IsDir() {
			sz = info.Size()
		}
		kind := "other"
		switch {
		case d.Type()&os.ModeSymlink != 0:
			kind = "symlink"
		case d.IsDir():
			kind = "dir"
		case d.Type().IsRegular():
			kind = "file"
		}
		entries = append(entries, map[string]any{
			"name":  d.Name(),
			"path":  fullPath,
			"type":  kind,
			"size":  sz,
			"mtime": mt,
		})
	}
	if recursive {
		err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if p == path {
				return nil
			}
			info, _ := d.Info()
			appendEntry(p, d, info)
			if maxEntries > 0 && len(entries) >= maxEntries {
				return io.EOF
			}
			return nil
		})
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
	} else {
		ds, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		for _, d := range ds {
			info, _ := d.Info()
			appendEntry(filepath.Join(path, d.Name()), d, info)
			if maxEntries > 0 && len(entries) >= maxEntries {
				break
			}
		}
	}
	return map[string]any{"path": path, "entries": entries}, nil
}

func (s *Service) Glob(pattern string, maxMatches int) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if maxMatches > 0 && len(matches) > maxMatches {
		matches = matches[:maxMatches]
	}
	return matches, nil
}

func (s *Service) Stat(path string) (map[string]any, error) {
	st, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{"path": path, "exists": false}, nil
		}
		return nil, err
	}
	kind := "other"
	switch {
	case st.Mode()&os.ModeSymlink != 0:
		kind = "symlink"
	case st.IsDir():
		kind = "dir"
	case st.Mode().IsRegular():
		kind = "file"
	}
	out := map[string]any{
		"path":   path,
		"exists": true,
		"type":   kind,
		"size":   st.Size(),
		"mtime":  st.ModTime().UnixMilli(),
		"mode":   st.Mode().String(),
	}
	if kind == "symlink" {
		if target, err := os.Readlink(path); err == nil {
			out["symlink_target"] = target
		}
	}
	return out, nil
}

func DecodeContent(content, encoding string) ([]byte, error) {
	if encoding == "base64" {
		return base64.StdEncoding.DecodeString(content)
	}
	return []byte(content), nil
}

func EncodeNow() int64 {
	return time.Now().UTC().UnixMilli()
}
