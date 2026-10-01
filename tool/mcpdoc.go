package tool

// MCPToolDescription is the reusable command description projected into the
// official MCP Go SDK's Tool JSON shape. The Bashy MCP server can use the same
// fields for tools/list; man emits them for agents working without a server.
type MCPToolDescription struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type MCPToolCall struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"params"`
}

type MCPCommandDocumentation struct {
	Tool MCPToolDescription `json:"tool"`
	Call MCPToolCall        `json:"call"`
}

// DocumentAsMCP gives a direct per-command tool its agent-readable metadata
// and a syntactically valid tools/call example. The server supplies execution
// and policy; this value contains no authority to run the command.
func DocumentAsMCP(name, description string) MCPCommandDocumentation {
	result := MCPCommandDocumentation{
		Tool: MCPToolDescription{
			Name: name, Description: description,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"args": map[string]any{
						"type": "array", "items": map[string]any{"type": "string"},
						"description": "Command arguments in order, including options and operands.",
					},
				},
				"additionalProperties": false,
			},
		},
	}
	result.Call.JSONRPC = "2.0"
	result.Call.Method = "tools/call"
	result.Call.Params.Name = name
	result.Call.Params.Arguments = map[string]any{"args": []string{}}
	return result
}
