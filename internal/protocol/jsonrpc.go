package protocol

import "encoding/json"

const Version = "2.0"

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string     `json:"jsonrpc"`
	ID      any        `json:"id,omitempty"`
	Result  any        `json:"result,omitempty"`
	Error   *RPCError  `json:"error,omitempty"`
}

type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func ErrorResponse(id any, code int, msg string, data any) Response {
	return Response{
		JSONRPC: Version,
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: msg,
			Data:    data,
		},
	}
}

const (
	ErrInvalidParams        = -32602
	ErrMethodNotFound       = -32601
	ErrUnauthorized         = -32001
	ErrForbiddenPath        = -32002
	ErrTimeout              = -32003
	ErrOutputLimitExceeded  = -32004
	ErrProcessNotFound      = -32005
	ErrConcurrencyConflict  = -32006
	ErrUnsupportedCapability = -32007
	ErrResourceLimit        = -32008
)
