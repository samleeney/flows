# Progress

## Completed

### Phase 1.1: Parser
- [x] Core model types (`pkg/model/`)
- [x] Markdown parser using goldmark (`pkg/parser/`)
- [x] Tests: 6 passing

### Phase 1.2: Serializer
- [x] Flow-to-markdown serializer (`pkg/serializer/`)
- [x] Round-trip tests (parse -> serialize -> parse): 3 passing

### Phase 1.3: Validator
- [x] Reference checking, cycle detection, max_runs enforcement (`pkg/validator/`)
- [x] Tests: 8 passing

### Phase 1.4: Runtime + CLI
- [x] Reactive dataflow scheduler (`pkg/runtime/`)
- [x] Parallel execution, conditional branching, loops
- [x] Bash function node executor
- [x] Pluggable executor registry
- [x] CLI entry point (`cmd/flow/`)
- [x] `flow validate` command
- [x] `flow run` command with --input, --verbose, --dry-run, --output
- [x] Tests: 5 passing

### Phase 2: Static visualization
- [x] Mermaid diagram generation (`pkg/viz/`)
- [x] `flow viz` command
- [x] Tests: 3 passing

### Phase 3.1: Editor backend
- [x] HTTP/WebSocket server (`pkg/editor/`)
- [x] GET/PUT /api/flow JSON API
- [x] WebSocket bi-directional sync
- [x] File watcher for external edits
- [x] `flow chart` command with --port, --no-open, --ui-dir
- [x] Tests: 3 passing

### Phase 3.2: Editor frontend
- [x] React Flow UI (`ui/`)
- [x] Custom AgentNode and FunctionNode components
- [x] WebSocket hook for real-time sync
- [x] Dagre auto-layout
- [x] Grid snapping, drag-and-drop
- [x] Position changes sync back to server and .md file
- [x] Built to ui/dist/

### Phase 3.3: Bi-directional sync
- [x] Browser -> file: drag/connect updates .md file via WebSocket
- [x] File -> browser: file watcher detects changes, pushes to clients

**Phase 3 milestone reached**: `flow chart` opens a functional visual editor.

Total: 28 tests passing across 6 packages.

## Next

### Phase 4: Polish and extensibility
- Python language executor
- Embed ui/dist in Go binary via embed.FS
- Cross-compilation
