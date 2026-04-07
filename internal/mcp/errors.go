package mcp

import "github.com/regiellis/mcp-searxng-go/pkg/types"

const (
	errParse          = -32700
	errInvalidRequest = -32600
	errMethodNotFound = -32601
	errInvalidParams  = -32602
	errInternal       = -32603
)

func responseError(id any, code int, message string, data map[string]any) types.JSONRPCResponse {
	return types.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &types.JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}
