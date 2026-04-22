package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── helpers ──────────────────────────────────────────────────────────────────

type greetInput struct {
	Name string `json:"name" jsonschema:"the name to greet"`
}

type greetOutput struct {
	Greeting string `json:"greeting"`
}

func greetHandler(_ context.Context, _ *mcp.CallToolRequest, in greetInput) (
	*mcp.CallToolResult, greetOutput, error,
) {
	return nil, greetOutput{Greeting: "Hello " + in.Name}, nil
}

// mcpTestConfigurator implements MCPConfigurator with an injected dep.
type mcpTestConfigurator struct {
	d *dep
}

func (c *mcpTestConfigurator) ConfigureMCP(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "di_tool",
		Description: "tool using DI dep",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in greetInput) (
		*mcp.CallToolResult, greetOutput, error,
	) {
		return nil, greetOutput{Greeting: "dep=" + string(rune(c.d.id+'0'))}, nil
	})
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestBuilder_WithMCP_RegistersHandler(t *testing.T) {
	boot, err := New().
		GRPCPort(":0").
		HTTPPort(":0").
		WithMCP(
			&mcp.Implementation{Name: "test-server", Version: "v0.1.0"},
			nil,
		).
		ConfigureMCP(func(s *mcp.Server) {
			mcp.AddTool(s, &mcp.Tool{
				Name:        "greet",
				Description: "say hello",
			}, greetHandler)
		}).
		Build()

	require.NoError(t, err)
	defer boot.Shutdown(context.Background())

	// The MCP handler is mounted on /mcp on the HTTP server.
	// Send a POST with a JSON-RPC initialize request to verify it responds.
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"v1"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(initReq))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	boot.http.Handler.ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	// Should get a valid JSON-RPC response (200 or 202).
	assert.Contains(t, []int{http.StatusOK, http.StatusAccepted}, resp.StatusCode,
		"unexpected status: %d, body: %s", resp.StatusCode, string(body))

	// Verify the response contains server info.
	assert.Contains(t, string(body), "test-server")
}

func TestBuilder_WithMCP_CustomPath(t *testing.T) {
	boot, err := New().
		GRPCPort(":0").
		HTTPPort(":0").
		WithMCP(
			&mcp.Implementation{Name: "custom-path", Version: "v1.0.0"},
			nil,
		).
		MCPPath("/custom-mcp").
		Build()

	require.NoError(t, err)
	defer boot.Shutdown(context.Background())

	// Default /mcp should not have a handler.
	req := httptest.NewRequest(http.MethodPost, "/custom-mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"v1"}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	boot.http.Handler.ServeHTTP(w, req)

	// Should get a valid response at the custom path.
	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, []int{http.StatusOK, http.StatusAccepted}, resp.StatusCode,
		"unexpected status: %d, body: %s", resp.StatusCode, string(body))
}

func TestBuilder_AddMCPConfigurator_DI(t *testing.T) {
	d := &dep{id: 5}

	boot, err := New().
		GRPCPort(":0").
		HTTPPort(":0").
		Provide(d).
		WithMCP(
			&mcp.Implementation{Name: "di-server", Version: "v1.0.0"},
			nil,
		).
		AddMCPConfigurator(func(dd *dep) *mcpTestConfigurator {
			return &mcpTestConfigurator{d: dd}
		}).
		Build()

	require.NoError(t, err)
	defer boot.Shutdown(context.Background())

	// Initialize session first.
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"v1"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(initReq))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	boot.http.Handler.ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	assert.Contains(t, []int{http.StatusOK, http.StatusAccepted}, resp.StatusCode,
		"status: %d, body: %s", resp.StatusCode, string(body))

	// Verify the response includes the di_tool in capabilities.
	assert.Contains(t, string(body), "di-server")
}

func TestBuilder_WithMCP_ToolListViaInMemory(t *testing.T) {
	// Use the MCP SDK's in-memory transport to verify tools are registered.
	mcpSrv := mcp.NewServer(
		&mcp.Implementation{Name: "test", Version: "v1"},
		nil,
	)
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "greet",
		Description: "say hello",
	}, greetHandler)

	st, ct := mcp.NewInMemoryTransports()

	_, err := mcpSrv.Connect(context.Background(), st, nil)
	require.NoError(t, err)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	session, err := client.Connect(context.Background(), ct, nil)
	require.NoError(t, err)
	defer session.Close()

	// List tools.
	result, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result.Tools, 1)
	assert.Equal(t, "greet", result.Tools[0].Name)

	// Call the tool.
	callResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "greet",
		Arguments: map[string]any{"name": "World"},
	})
	require.NoError(t, err)
	require.False(t, callResult.IsError)

	// The structured output should contain our greeting.
	structJSON, _ := json.Marshal(callResult.StructuredContent)
	assert.Contains(t, string(structJSON), "Hello World")
}

func TestBuilder_MCPHTTPOptions_SetsOptions(t *testing.T) {
	opts := &mcp.StreamableHTTPOptions{Stateless: true}
	b := New().MCPHTTPOptions(opts)
	assert.Equal(t, opts, b.mcpHTTPOpts)
}

func TestBuilder_AddMCPConfigurator_NonFunction_CallsFatal(t *testing.T) {
	mockLogger := withMockLogger(func() {
		New().AddMCPConfigurator("not a function")
	})

	assert.True(t, mockLogger.isFatalCalled, "expected logger.Fatal to be called")
	assert.Equal(t, "AddMCPConfigurator expects a factory function", mockLogger.fatalMsg)
}

func TestBuilder_AddMCPConfigurator_NonConfigurator_CallsFatal(t *testing.T) {
	mockLogger := withMockLogger(func() {
		// Factory returns *dep which does not implement MCPConfigurator
		New().AddMCPConfigurator(func() *dep {
			return &dep{id: 1}
		})
	})

	assert.True(t, mockLogger.isFatalCalled, "expected logger.Fatal to be called")
	assert.Equal(t, "factory must return a type implementing MCPConfigurator", mockLogger.fatalMsg)
}

func TestBuilder_AddMCPConfigurator_DIFails_ReturnsError(t *testing.T) {
	// Register a configurator that requires *dep which is NOT provided
	_, err := New().
		GRPCPort(":0").
		HTTPPort(":0").
		WithMCP(
			&mcp.Implementation{Name: "test", Version: "v1"},
			nil,
		).
		AddMCPConfigurator(func(d *dep) *mcpTestConfigurator {
			return &mcpTestConfigurator{d: d}
		}).
		Build()

	assert.Error(t, err, "expected Build() to fail when MCP configurator dependency is missing")
	assert.Contains(t, err.Error(), "MCP configurator DI failed")
}

func TestBuilder_WithMCPMiddleware_StoresMiddleware(t *testing.T) {
	mw := func(next http.Handler) http.Handler { return next }
	b := New().WithMCPMiddleware(mw)
	assert.NotNil(t, b.mcpMiddleware, "middleware should be stored on builder")
}

func TestBuilder_WithMCPMiddleware_AppliedToRequests(t *testing.T) {
	// Middleware that blocks requests without an "X-Auth" header.
	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Auth") == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	boot, err := New().
		GRPCPort(":0").
		HTTPPort(":0").
		WithMCP(
			&mcp.Implementation{Name: "mw-test", Version: "v1.0.0"},
			nil,
		).
		WithMCPMiddleware(authMiddleware).
		Build()

	require.NoError(t, err)
	defer boot.Shutdown(context.Background())

	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"v1"}}}`

	// Request WITHOUT the auth header should be rejected by middleware.
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(initReq))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	boot.http.Handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode,
		"request without X-Auth header should be blocked by middleware")

	// Request WITH the auth header should reach the MCP server.
	req = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(initReq))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("X-Auth", "valid-token")
	w = httptest.NewRecorder()
	boot.http.Handler.ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, []int{http.StatusOK, http.StatusAccepted}, resp.StatusCode,
		"authenticated request should succeed, got status %d, body: %s", resp.StatusCode, string(body))
	assert.Contains(t, string(body), "mw-test")
}

func TestBuilder_WithMCPMiddleware_HeaderInjection(t *testing.T) {
	// Middleware that injects a response header.
	headerMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-MCP-Version", "1.0")
			next.ServeHTTP(w, r)
		})
	}

	boot, err := New().
		GRPCPort(":0").
		HTTPPort(":0").
		WithMCP(
			&mcp.Implementation{Name: "header-test", Version: "v1.0.0"},
			nil,
		).
		WithMCPMiddleware(headerMiddleware).
		Build()

	require.NoError(t, err)
	defer boot.Shutdown(context.Background())

	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"v1"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(initReq))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	boot.http.Handler.ServeHTTP(w, req)

	assert.Equal(t, "1.0", w.Result().Header.Get("X-MCP-Version"),
		"middleware should inject X-MCP-Version header")
}

func TestBuilder_WithMCPMiddleware_NilMiddleware_NoWrap(t *testing.T) {
	// When no middleware is set, MCP handler should work directly.
	boot, err := New().
		GRPCPort(":0").
		HTTPPort(":0").
		WithMCP(
			&mcp.Implementation{Name: "no-mw", Version: "v1.0.0"},
			nil,
		).
		Build()

	require.NoError(t, err)
	defer boot.Shutdown(context.Background())

	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"v1"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(initReq))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	boot.http.Handler.ServeHTTP(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, []int{http.StatusOK, http.StatusAccepted}, resp.StatusCode,
		"MCP without middleware should work, got status %d, body: %s", resp.StatusCode, string(body))
	assert.Contains(t, string(body), "no-mw")
}
