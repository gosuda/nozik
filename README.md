# Nozik — AI Music Agent for Gozik

Nozik is a GTK dashboard AI agent that talks to the [Gozik](https://github.com/gg582/gozik) music player over gRPC, stores your music taste in a local SQLite database, and exposes an MCP (Model Context Protocol) server for external tool integration.

## Features

- **AI provider strategy pattern**
  - Gemini (Tested, OK)
  - Kimi / Moonshot AI (WIP)
  - ChatGPT / OpenAI-compatible(WIP)
  - Grok / xAI-compatible(Untested, followed official docs)
  - Others — custom endpoint + Lua response parser
- **Taste database** (SQLite)
  - Records every approved play with emotion tags
  - Import / export JSON
  - Join two taste DBs
  - Discovery engine: `Deep Diver` (conservative similarity) and `Pioneer` (diverse / anomaly)
- **Emotion-aware curation**
  - Depressed → conservative, familiar picks
  - Excited → can surface pioneer recommendations
- **MCP server** (`nozik mcp`)
  - `nozik_db_search` — search/curate the taste DB
  - `nozik_gozik_installation` — locate the Gozik binary and gRPC address
- **GTK dashboard** styled with Gozik's cosmic CSS and a color-inverted Nozik icon

## Install

Download the installer for your platform from [GitHub Releases](https://github.com/gosuda/nozik/releases/latest), then run it and choose a version. The installer downloads the matching application payload and installs Nozik for the current user.

On Linux, a headless installation is also available:

```bash
chmod +x nozik-installer-linux-arm64
./nozik-installer-linux-arm64 --headless --version latest
```

## Build

```bash
cd nozik
go build -o nozik ./cmd/nozik
```

## Run

```bash
# GUI dashboard
./nozik

# MCP server (stdio)
./nozik mcp

# Help
./nozik --help
```

## Configuration

Open the Settings dashboard and paste the API key for your chosen provider.
Keys are saved in `~/.config/nozik/settings.json`.
If a key is left blank, Nozik falls back to the matching environment variable.
Use the **Model** dropdown to choose a specific model; click **Refresh models** to fetch the list available for the current API key:

| Provider | Settings field | Env fallback |
|----------|----------------|--------------|
| Gemini   | Gemini API key | `GEMINI_API_KEY` or `GOOGLE_API_KEY` |
| ChatGPT  | OpenAI API key | `OPENAI_API_KEY` |
| Grok     | Grok API key   | `GROK_API_KEY` |
| Kimi     | Kimi API key   | `KIMI_API_KEY` |
| Others   | Custom Lua script | — |

Gozik gRPC address defaults to `127.0.0.1:50051`. Set `GOZIK_GRPC_ADDR` to override.

## Project Layout

```
nozik/
├── cmd/nozik          # Entry point (GUI + MCP)
├── internal/ai        # Strategy pattern AI providers
├── internal/config    # Paths and environment helpers
├── internal/curation  # Emotion / discovery logic
├── internal/db        # SQLite taste database
├── internal/gzk       # Gozik gRPC client
├── internal/mcp       # Stdio MCP server
├── internal/model     # Shared types
├── internal/ui        # GTK dashboard + settings dialog
└── assets/            # CSS, inverted icon, example Lua script
```

## Custom Provider Example

See `assets/examples/custom_provider.lua.example`. A custom script must define:

```lua
function build_request(user_message, emotion)
    return { url=..., method=..., headers=..., body=... }
end

function parse_response(response_body_text)
    return { { title=..., artist=..., provider_id=..., provider_track_id=... } }
end
```
