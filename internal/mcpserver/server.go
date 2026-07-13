// Package mcpserver implements just enough of the Model Context Protocol
// (JSON-RPC 2.0 over stdio) to expose Aegis pipeline as a tool any MCP
// client can call — Claude Desktop, Claude Code, or your own agent.
//
// This is hand-rolled on purpose: no external SDK, so `go build` never
// needs network access, and the whole protocol surface fits in one file
// you can actually read.
//
// IMPORTANT: only JSON-RPC messages may ever be written to stdout — that
// is the transport. All logging goes to stderr.
package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/ValeryCherneykin/aegis/internal/pipeline"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const toolName = "request_refund"

// Serve blocks, reading JSON-RPC requests from in and writing responses
// to out until in is closed (EOF), which is how MCP clients signal
// shutdown over stdio.
func Serve(pl *pipeline.Pipeline, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			fmt.Fprintf(os.Stderr, "aegis: bad json-rpc line: %v\n", err)
			continue
		}
		handle(pl, req, out)
	}
	return scanner.Err()
}

func handle(pl *pipeline.Pipeline, req rpcRequest, out io.Writer) {
	isNotification := len(req.ID) == 0

	switch req.Method {
	case "initialize":
		writeResult(out, req.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]interface{}{"name": "aegis", "version": "0.1.0"},
		})

	case "notifications/initialized":
		// notification, no response expected

	case "tools/list":
		writeResult(out, req.ID, map[string]interface{}{
			"tools": []map[string]interface{}{
				{
					"name":        toolName,
					"description": "Evaluate and act on a pending refund request. The tool NEVER returns raw amounts or PII — only category labels and the final action.",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"request_id": map[string]interface{}{
								"type":        "string",
								"description": "Identifier of the pending refund request (never a dollar amount).",
							},
						},
						"required": []string{"request_id"},
					},
				},
			},
		})

	case "tools/call":
		handleToolCall(pl, req, out)

	default:
		if !isNotification {
			writeError(out, req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
		}
	}
}

func handleToolCall(pl *pipeline.Pipeline, req rpcRequest, out io.Writer) {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeError(out, req.ID, -32602, "invalid params")
		return
	}
	if params.Name != toolName {
		writeError(out, req.ID, -32602, fmt.Sprintf("unknown tool %q", params.Name))
		return
	}
	requestID, _ := params.Arguments["request_id"].(string)
	if requestID == "" {
		writeError(out, req.ID, -32602, "request_id is required")
		return
	}

	trace, err := pl.Run(context.Background(), requestID)
	if err != nil {
		writeToolResult(out, req.ID, fmt.Sprintf("error: %v", err), true)
		return
	}

	// This is the ONLY thing that goes back into the agent's context.
	// Deliberately built from categories + decision only — never from
	// executor.Result.Summary, which contains real amounts/IDs and is
	// only ever shown on the human/audit side.
	summary := fmt.Sprintf(
		"Categories observed: %s\nFinal action: %s\nReason: %s\nOverridden by hard rule: %t\n(No raw amounts, names or IDs were ever visible to you.)",
		strings.Join(sortedCategoryLines(trace.Categories), ", "),
		trace.FinalDecision.Action,
		trace.FinalDecision.Reason,
		trace.Overridden,
	)
	writeToolResult(out, req.ID, summary, false)
}

func writeResult(out io.Writer, id json.RawMessage, result interface{}) {
	resp := rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
	enc := json.NewEncoder(out)
	_ = enc.Encode(resp)
}

func writeError(out io.Writer, id json.RawMessage, code int, msg string) {
	resp := rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
	enc := json.NewEncoder(out)
	_ = enc.Encode(resp)
}

func writeToolResult(out io.Writer, id json.RawMessage, text string, isError bool) {
	writeResult(out, id, map[string]interface{}{
		"content": []map[string]interface{}{{"type": "text", "text": text}},
		"isError": isError,
	})
}

func sortedCategoryLines(categories map[string]string) []string {
	keys := make([]string, 0, len(categories))
	for k := range categories {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", k, categories[k]))
	}
	return lines
}
