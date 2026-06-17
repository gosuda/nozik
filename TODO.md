# Nozik AI Music Agent — Implementation TODO

Nozik is an AI-agent GTK dashboard that talks to the Gozik music player over gRPC.
It stores the user's music taste in a local SQLite DB and exposes an MCP server
so external tools can query/curate that taste.

## 1. Environment & Assets
- [x] Create `nozik/` Go module layout
- [x] Copy Gozik cosmic GTK theme (`gozik.css`) into `assets/ui/nozik.css`
- [x] Generate color-inverted Gozik icon as `assets/icons/nozik.png`

## 2. AI Provider Strategy Pattern
Interface: `internal/ai.AIProviderStrategy`
- [x] `GeminiStrategy` — Google Generative Language API
- [x] `KimiStrategy` — Moonshot AI (`api.moonshot.cn`) chat completions
- [x] `OpenAIStrategy` — OpenAI-compatible chat completions (stub/ready)
- [x] `GrokStrategy` — xAI Grok-compatible chat completions (stub/ready)
- [x] `LuauCustomStrategy` — "Others" provider: custom endpoint + Luau response parser

## 3. Database & Curation
SQLite layer in `internal/db`:
- [x] Schema: `plays` with emotion tags, play counts, weights
- [x] Record plays / taste entries
- [x] Import / Export JSON taste files
- [x] DB Join (merge two taste DBs)
- [x] Discovery engine:
  - `Deep Diver` — conservative high-similarity search
  - `Pioneer` — low-play / diverse anomaly recommendations
- [x] Emotional-state curation (`Conservative` when depressed, `Adaptive` when excited)

## 4. Gozik gRPC Integration
- [x] gRPC client wrapper using Gozik's generated protobuf packages
- [x] Helpers: `ListProviders`, `SearchTracks`, `AddProviderTrack`, `AddToQueue`

## 5. MCP Server
`nozik mcp` runs a stdio Model Context Protocol server with tools:
- [x] `nozik_db_search` — DB lookup / curation by emotion and discovery mode
- [x] `nozik_gozik_installation` — locate `gozik` binary / gRPC address

## 6. GTK Dashboard
- [x] Main window styled with Gozik CSS
- [x] Chat view (Nozik / User / MCP messages)
- [x] Horizontal recommendation list
- [x] Approve / Modify / Deny action group
- [x] Active songlist view component
- [x] Settings dashboard:
  - Provider selection (Gemini, ChatGPT, Grok, Kimi, Others)
  - Custom Luau script / endpoint editor

## 7. Build & Run
- [x] `go build ./cmd/nozik` compiles
- [x] `--help` prints usage

## Notes / Future Improvements
- Richer natural-language emotion parsing (currently keyword-based).
- Real album expansion via `GetProviderPlaylistDetails`.
- Streaming AI responses for lower latency.
- More advanced audio feature vectors for similarity.
