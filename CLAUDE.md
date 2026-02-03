# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```bash
# Development
make dev              # Run client with hot reload
make run-server       # Run server locally (port 8080, token: dev-token)

# Build
make build            # Build client for current platform
make server           # Build server for current platform
make build-all        # Build all platforms

# Dependencies
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails doctor          # Check Wails environment
```

## Architecture

This is a Claude Code history sync tool with client-server architecture:

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Desktop Client │────▶│  Sync Server    │◀────│  Desktop Client │
│  (Wails App)    │     │  (Multi-tenant) │     │  (Wails App)    │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

### Two Separate Applications

1. **Client** (`main.go`) - Wails desktop app with WebView UI
   - Embeds `assets/index.html` as the UI
   - `internal/service/sync.go` - Sync logic, file scanning, HTTP client
   - `internal/config/config.go` - Client configuration (`~/.claude/sync-config.json`)

2. **Server** (`cmd/server/main.go`) - HTTP server with multi-tenant support
   - `internal/service/server.go` - Multi-tenant sync server, tenant management
   - `internal/service/admin_ui.go` + `admin.html` - Web admin console at `/admin`

### Key Data Flow

- Client scans `~/.claude/projects/` for files
- Files are hashed (SHA256) and compared with server
- Only changed files are uploaded/downloaded
- Path mappings allow syncing between machines with different directory structures

### Multi-Tenancy

- Each tenant has isolated data in `{dataDir}/tenants/{tenantID}/`
- Tenants are identified by unique tokens
- Server config stored in `{dataDir}/config.json`

## Frontend

The client UI is a single HTML file (`assets/index.html`) embedded via `//go:embed`. It communicates with Go backend through Wails bindings (methods on `App` struct in `main.go`).

The server admin UI is embedded in `internal/service/admin.html` and served at `/admin`.
