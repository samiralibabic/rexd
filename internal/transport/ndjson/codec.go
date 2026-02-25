package ndjson

import (
	"bufio"
	"encoding/json"
	"io"
	"sync"
)

type Decoder struct {
	reader *bufio.Reader
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{reader: bufio.NewReader(r)}
}

func (d *Decoder) Decode(v any) error {
	line, err := d.reader.ReadBytes('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(line, v)
}

type Encoder struct {
	writer io.Writer
	mu     sync.Mutex
}

func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{writer: w}
}

func (e *Encoder) Encode(v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := e.writer.Write(payload); err != nil {
		return err
	}
	_, err = e.writer.Write([]byte("\n"))
	return err
}
