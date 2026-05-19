# joplin-mcp-go

A fast, dependency-light MCP server for [Joplin](https://joplinapp.org/) written in Go using the [official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk).

Built as a reliable replacement for the Python-based Joplin MCP server, which suffers from dependency breakage due to infrequent maintenance.

## Features

- **24 tools** covering notes, notebooks, tags, and search
- **Single binary** — no runtime dependencies, no Python, no Node.js
- **Official MCP SDK** — `github.com/modelcontextprotocol/go-sdk` (maintained by Google + Anthropic)
- **Stdio transport** — works with Claude Desktop, Claude Code, Cursor, etc.
- **LLM-friendly addressing** — tools accept either a Joplin ID or a title/path for notebooks (`"Projects/Work"`) and a title for tags. Ambiguous names return a structured error listing all candidates with their IDs.
- **Todo filtering** — `find_notes`, `get_notebook_notes`, and `get_tag_notes` all accept `task` (`"todo"` / `"note"`) and `completed` (true / false) filters, implemented via Joplin's native search operators.
- **Partial note editing** — `patch_note` applies surgical edits (text replacement, line operations, insert/append) without rewriting the entire body, saving tokens on large notes.

## Tools (24)

### Notes (8)
| Tool | Description |
|------|-------------|
| `find_notes` | Full-text search over notes. Use `*` to list all. Supports `task` and `completed` filters. |
| `get_note` | Get a single note by ID; control returned fields to skip the body when you just need metadata |
| `create_note` | Create a note; target notebook by `parent_id` or `notebook_name` |
| `update_note` | Update note properties (title, body, todo state, move by id or name). Replaces entire body. |
| `patch_note` | Apply partial edits to a note's body via text / line / position operations (see below) |
| `grep_note` | Search inside a note's body; returns matching lines with line numbers and surrounding context |
| `delete_note` | Delete note (trash or permanent) |
| `get_tags_by_note` | Get all tags attached to a note |

### Notebooks (7)
| Tool | Description |
|------|-------------|
| `list_notebooks` | List all notebooks as a tree |
| `resolve_notebook` | Resolve a title or path to `{id, title, path}`; errors with candidates if ambiguous |
| `get_notebook` | Get a notebook by `id` or `notebook_name` |
| `get_notebook_notes` | Paginated notes in a notebook, with `task` / `completed` filters |
| `create_notebook` | Create a notebook; parent by `parent_id` or `parent_notebook_name` |
| `update_notebook` | Rename or move a notebook (target and parent both accept id or name) |
| `delete_notebook` | Delete a notebook by `id` or `notebook_name` |

### Tags (7)
| Tool | Description |
|------|-------------|
| `list_tags` | List all tags |
| `resolve_tag` | Resolve a tag title to `{id, title}`; errors with candidates if ambiguous |
| `get_tag_notes` | Paginated notes with a tag (by `tag_id` or `tag_name`), with `task` / `completed` filters |
| `create_tag` | Create a new tag |
| `tag_note` | Attach a tag to a note (by `tag_id` or `tag_name`) |
| `untag_note` | Remove a tag from a note (by `tag_id` or `tag_name`) |
| `delete_tag` | Delete a tag (by `id` or `tag_name`) |

### Utility (2)
| Tool | Description |
|------|-------------|
| `search` | General-purpose Joplin search with full query syntax and `type` filter (`note` / `folder` / `tag`) |
| `ping` | Check if Joplin is running |

## Partial note editing (`patch_note`)

`patch_note` lets you edit a note's body without sending the entire content. The server fetches the current body, applies your operations in order, then saves the result — all in a single tool call.

Three families of operations are available:

**Text-based** (anchor text must appear exactly once by default; set `count` for multiple):

| Operation | Fields | Description |
|-----------|--------|-------------|
| `replace` | `old`, `new`, `count` | Find `old` text and swap with `new` |
| `delete` | `target`, `count` | Remove `target` text |
| `insert_before` | `anchor`, `content`, `count` | Insert `content` immediately before `anchor` |
| `insert_after` | `anchor`, `content`, `count` | Insert `content` immediately after `anchor` |

The optional `count` field controls how many occurrences to act on:
- **omit or 0** → default 1 (text must be unique; error if multiple matches)
- **N > 1** → act on the first N occurrences (error if fewer than N exist)
- **-1** → act on ALL occurrences

**Position-based:**

| Operation | Fields | Description |
|-----------|--------|-------------|
| `prepend` | `content` | Add `content` at the top of the body |
| `append` | `content` | Add `content` at the bottom of the body |

**Line-based** (1-based inclusive line numbers):

| Operation | Fields | Description |
|-----------|--------|-------------|
| `replace_lines` | `start`, `end`, `content` | Replace lines `start` through `end` with `content` |
| `insert_at_line` | `line`, `content` | Insert `content` before line `line` |
| `delete_lines` | `start`, `end` | Delete lines `start` through `end` |

Operations are applied sequentially — each sees the body as modified by the previous one. For text-based ops with the default `count` of 1, if the anchor text matches zero or more than one location, the operation fails with the match count so you can add more surrounding context and retry. Set `count` to allow multiple matches.

Example — append a section and fix a typo in one call:

```json
{
  "id": "abc123def456...",
  "operations": [
    {"op": "append", "content": "\n## New Section\n\nContent here."},
    {"op": "replace", "old": "teh old text", "new": "the old text"}
  ]
}
```

## Query syntax (used by `find_notes` and `search`)

Joplin's search supports:

- `"exact phrase"` — phrase match
- `title:word` / `body:word` — field-scoped search
- `-word` — exclude
- `word1 OR word2` — either
- `tag:tagname` — tagged notes
- `notebook:"Name"` — in a specific notebook
- `type:note` / `type:todo` — item type
- `iscompleted:0` / `iscompleted:1` — todo completion
- `*` — wildcard (alone = everything)

## Name resolution

Joplin allows two notebooks to share the same title under different parents, and in rare cases tags with the same title can exist. To keep tools LLM-friendly while staying safe, notebook- and tag-targeting tools accept **either** an ID **or** a name.

`notebook_name` is resolved in this order:

1. **32-hex ID** — returned as-is after verifying the notebook exists
2. **Exact path** — case-sensitive match on the full path (e.g. `"Projects/Work"`)
3. **Exact title** — case-sensitive match on title alone
4. **Case-insensitive title or path** — last-resort fallback

`tag_name` uses the same approach minus the path step (tags have no hierarchy).

If the name matches more than one, the tool returns an error listing all candidates. Example:

```
multiple notebooks match "meeting-notes". Use a full path or notebook_id. Candidates:
  - id=abc123...def  path="meeting-notes"
  - id=xyz789...uvw  path="Archive/meeting-notes"
```

When both an ID and a name are passed, the ID wins. You can also call `resolve_notebook` / `resolve_tag` directly to check whether a name is unique before using it.

## Prerequisites

1. **Go 1.22+** installed (recent SDK versions may require Go 1.24+ or 1.25+; adjust `go.mod` if needed)
2. **Joplin Desktop** running with Web Clipper service enabled
3. **Authorization token** from Joplin > Options > Web Clipper > Advanced Options

## Build

```bash
cd path/to/joplin-mcp-go

# Download dependencies
go mod tidy

# Build (Windows)
go build -o joplin-mcp-go.exe .

# Build (macOS/Linux)
go build -o joplin-mcp-go .
```

## Configuration

The server reads two environment variables:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `JOPLIN_TOKEN` | Yes | — | Web Clipper authorization token |
| `JOPLIN_PORT` | No | `41184` | Joplin API port |

There are three ways to supply these to the server, depending on which client you use.

### Option 1 — System environment variables (recommended for Claude Desktop)

Claude Desktop's `claude_desktop_config.json` does **not** expand `${VAR}` placeholders in the `env` block — values are passed as literal strings. The cleanest approach is to set `JOPLIN_TOKEN` once at the OS level and **omit the `env` block** entirely. Claude Desktop inherits the parent process's environment when `env` is not set.

**Windows:**
1. Open `System Properties → Advanced → Environment Variables`
2. Under **User variables**, click **New…**
3. Name: `JOPLIN_TOKEN`, Value: your token
4. Click OK, then **fully restart Claude Desktop** (quit from the tray, not just close the window)

**macOS / Linux:** add to `~/.zshrc` / `~/.bashrc`:
```bash
export JOPLIN_TOKEN="your-token-here"
```

Then the config is just:
```json
{
  "mcpServers": {
    "joplin": {
      "command": "/path/to/joplin-mcp-go"
    }
  }
}
```

Benefits: no token in the config file (safer for backups / git), one source of truth, works for other tools (CLI, scripts) that need the same token.

### Option 2 — Literal value in `claude_desktop_config.json`

Simple but puts the token in plaintext in the Claude config directory (`%APPDATA%\Claude\` on Windows, `~/Library/Application Support/Claude/` on macOS, `~/.config/Claude/` on Linux):

```json
{
  "mcpServers": {
    "joplin": {
      "command": "/path/to/joplin-mcp-go",
      "env": {
        "JOPLIN_TOKEN": "paste-token-directly-here"
      }
    }
  }
}
```

Fine for purely local setups. Avoid if you sync or back up the `Claude` config directory.

### Option 3 — `${VAR}` interpolation (Claude Code only)

Claude Code's `.mcp.json` (not Claude Desktop) supports environment variable expansion with `${VAR}` and `${VAR:-default}` syntax:

```json
{
  "mcpServers": {
    "joplin": {
      "command": "/path/to/joplin-mcp-go",
      "env": {
        "JOPLIN_TOKEN": "${JOPLIN_TOKEN}",
        "JOPLIN_PORT": "${JOPLIN_PORT:-41184}"
      }
    }
  }
}
```

Claude Code reads `JOPLIN_TOKEN` from the shell it was launched from. This does **not** work in Claude Desktop — the placeholder would be passed through as a literal string.

On Windows, use an absolute path with escaped backslashes, e.g. `"C:\\Tools\\joplin-mcp-go.exe"`.

## Architecture

```
joplin-mcp-go/
├── main.go                  # Entry point, config, server setup
├── go.mod                   # Go module definition
├── joplin/
│   ├── client.go            # Joplin REST API client (HTTP)
│   ├── resolver.go          # Notebook name/path resolution
│   └── tag_resolver.go      # Tag name resolution
└── tools/
    ├── register.go          # Tool registration hub, shared TextResult,
    │                        # resolveNotebookArg, resolveTagArg, todoFilter helpers
    ├── util.go              # Small internal utilities
    ├── notes.go             # find/get/create/update/delete_note, get_tags_by_note
    ├── patch.go             # patch_note (partial body editing)
    ├── grep.go              # grep_note (search inside a note's body)
    ├── folders.go           # list/resolve/get/create/update/delete_notebook,
    │                        # get_notebook_notes
    ├── tags.go              # list/resolve/create/delete_tag, tag/untag_note,
    │                        # get_tag_notes
    └── search.go            # search (raw query), ping
```

## License

MIT
