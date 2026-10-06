package tool

// MCPToolDescription is the reusable command description projected into the
// official MCP Go SDK's Tool JSON shape. The Bashy MCP server can use the same
// fields for tools/list; man emits them for agents working without a server.
type MCPToolDescription struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	InputSchema map[string]any      `json:"inputSchema"`
	Annotations *MCPToolAnnotations `json:"annotations,omitempty"`
	Meta        map[string]any      `json:"_meta,omitempty"`
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

// MCPToolAnnotations carries optional MCP tool hints, not execution authority.
// Pointers preserve explicit false values as distinct from unspecified hints.
type MCPToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

// ApplyEffects annotates a tool with its declared effects and supported OSes.
// Other metadata and unrelated annotation hints are preserved.
func (d *MCPToolDescription) ApplyEffects(effects []string, os []string) {
	if d.Meta == nil {
		d.Meta = make(map[string]any)
	}
	d.Meta["bashy.effects"] = effects
	d.Meta["bashy.os"] = os
	readOnly, destructive := true, false
	for _, effect := range effects {
		readOnly = readOnly && (effect == "pure" || effect == "read")
		destructive = destructive || effect == "destroy"
	}
	if d.Annotations == nil {
		d.Annotations = &MCPToolAnnotations{}
	}
	d.Annotations.ReadOnlyHint = &readOnly
	d.Annotations.DestructiveHint = &destructive
}

// DocumentAsMCPSchema projects documentation with a declared argument schema.
// The example has an empty argument object for the caller to fill in.
func DocumentAsMCPSchema(name, description string, s ArgSchema) MCPCommandDocumentation {
	result := DocumentAsMCP(name, description)
	result.Tool.InputSchema = s.JSONSchema()
	result.Call.Params.Arguments = map[string]any{}
	return result
}
