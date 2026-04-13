package server

import "github.com/modelcontextprotocol/go-sdk/mcp"

// MCPConfigurator is implemented by types that configure an MCP server
// with tools, resources, prompts, etc. It is the MCP equivalent of
// RestController – register a factory via Builder.AddMCPConfigurator
// and go-api-boot will inject dependencies and call ConfigureMCP during Build.
type MCPConfigurator interface {
	ConfigureMCP(s *mcp.Server)
}
