package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

var version = "dev"

const protocolVersion = "2024-11-05"

func main() {
	server := &MCPServer{}
	server.Run(os.Stdin, os.Stdout)
}

type MCPServer struct{}

func (s *MCPServer) Run(in io.Reader, out io.Writer) {
	scanner := bufio.NewScanner(in)
	buf := make([]byte, 1024*1024) // 1MB buffer
	scanner.Buffer(buf, len(buf))

	encoder := json.NewEncoder(out)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			fmt.Fprintf(os.Stderr, "invalid JSON-RPC: %s\n", err)
			continue
		}

		resp := s.handle(req)
		if resp != nil {
			encoder.Encode(resp)
		}
	}
}

func (s *MCPServer) handle(req JSONRPCRequest) *JSONRPCResponse {
	switch req.Method {
	case "initialize":
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: InitializeResult{
				ProtocolVersion: protocolVersion,
				Capabilities: ServerCapability{
					Tools: &ToolsCapability{},
				},
				ServerInfo: ServerInfo{
					Name:    "airstrings-mcp",
					Version: version,
				},
			},
		}

	case "notifications/initialized":
		// No response for notifications
		return nil

	case "tools/list":
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: ToolsListResult{
				Tools: toolDefs,
			},
		}

	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &JSONRPCError{
					Code:    -32602,
					Message: "invalid params",
				},
			}
		}

		handler, ok := toolHandlers[params.Name]
		if !ok {
			return &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  errorResult(fmt.Sprintf("unknown tool: %s", params.Name)),
			}
		}

		result := handler(params.Arguments)
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  result,
		}

	default:
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    -32601,
				Message: fmt.Sprintf("method not found: %s", req.Method),
			},
		}
	}
}
