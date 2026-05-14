# Progress

## Completed

### Phase 1.1: Parser
- [x] Core model types (`pkg/model/`)
- [x] Markdown parser using goldmark (`pkg/parser/`)
- [x] Tests: 6 passing

### Phase 1.2: Serializer
- [x] Flow-to-markdown serializer (`pkg/serializer/`)
- [x] Round-trip tests: 3 passing

### Phase 1.3: Validator
- [x] Reference checking, cycle detection, max_runs enforcement (`pkg/validator/`)
- [x] Tests: 8 passing

### Phase 1.4: Runtime + CLI
- [x] Reactive dataflow scheduler (`pkg/runtime/`)
- [x] Parallel execution, conditional branching, loops
- [x] Bash function node executor
- [x] Pluggable executor registry
- [x] CLI: `flow run`, `flow validate`
- [x] Tests: 5 passing

### Phase 2: Static visualization
- [x] Mermaid diagram generation (`pkg/viz/`)
- [x] `flow viz` command
- [x] Tests: 3 passing

### Phase 3: Visual editor
- [x] HTTP/WebSocket server (`pkg/editor/`)
- [x] React Flow frontend (`ui/`)
- [x] Bi-directional sync (browser <-> file)
- [x] `flow chart` command
- [x] Tests: 3 passing

### Phase 4: Polish and extensibility
- [x] Python language executor with tests (4 passing)
- [x] Embedded UI via Go embed.FS (12MB single binary)
- [x] Makefile for build pipeline (build-ui, build-go, clean)
- [x] Server refactored to accept http.FileSystem for embedded/disk serving
- [x] HTTP LLM prompt executor for real prompt-node execution via Anthropic
      Messages or OpenAI Responses APIs
- [x] Runtime passes flow defaults and per-agent model/temperature overrides to
      agent-aware executors
- [x] `flow run` supports `--llm-provider`, `--model`, `--max-tokens`, and
      `--llm-timeout`; API keys come from `ANTHROPIC_API_KEY` or
      `OPENAI_API_KEY`
- [x] CLI-level E2E tests covering function pipelines, conditional branches,
      mixed bash/python flows, mocked LLM prompt execution, and missing input
      failures

**All phases complete.** Prompt nodes now execute against real LLM APIs when
configured with a model and API key. Function-only flows still run without LLM
credentials.

## Summary

CLI commands:
- `flow run <file>` — execute a flow
- `flow validate <file>` — validate without executing
- `flow viz <file>` — output Mermaid diagram
- `flow chart <file>` — open visual editor in browser

Build: `make build` produces a single binary with embedded frontend.

LLM configuration for prompt nodes:
- Set `defaults.model` in flow frontmatter or `model` on an individual agent.
- Set `ANTHROPIC_API_KEY` for Claude models or `OPENAI_API_KEY` for OpenAI
  models.
- Optional environment overrides: `FLOW_LLM_PROVIDER`, `FLOW_MODEL`,
  `FLOW_MAX_TOKENS`, `FLOW_LLM_TIMEOUT`, `ANTHROPIC_BASE_URL`,
  `ANTHROPIC_VERSION`, `OPENAI_BASE_URL`.
