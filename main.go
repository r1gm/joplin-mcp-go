// Joplin MCP Server - A Go-based MCP server for the Joplin note-taking app.
//
// This server exposes the Joplin Data API as MCP tools, providing notes,
// notebooks, tags, and search functionality to MCP clients like Claude Desktop.
//
// Configuration via environment variables:
//
//	JOPLIN_TOKEN  - (required) Joplin Web Clipper authorization token
//	JOPLIN_PORT   - (optional) Joplin API port, default 41184
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/r1gm/joplin-mcp-go/joplin"
	"github.com/r1gm/joplin-mcp-go/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName    = "joplin-mcp-go"
	serverVersion = "1.0.0"
	defaultPort   = 41184
)

func main() {
	// Read configuration from environment
	token := os.Getenv("JOPLIN_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "ERROR: JOPLIN_TOKEN environment variable is required.")
		fmt.Fprintln(os.Stderr, "Get it from Joplin > Options > Web Clipper > Advanced Options > Authorization token")
		os.Exit(1)
	}

	port := defaultPort
	if portStr := os.Getenv("JOPLIN_PORT"); portStr != "" {
		var err error
		port, err = strconv.Atoi(portStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: Invalid JOPLIN_PORT value: %s\n", portStr)
			os.Exit(1)
		}
	}

	// Create the Joplin REST API client
	client := joplin.NewClient(port, token)

	// Create the MCP server
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    serverName,
			Version: serverVersion,
		},
		nil,
	)

	// Register all tools
	tools.RegisterAll(server, client)

	// Run over stdio (for Claude Desktop / MCP clients)
	fmt.Fprintf(os.Stderr, "%s v%s starting on Joplin port %d...\n", serverName, serverVersion, port)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
