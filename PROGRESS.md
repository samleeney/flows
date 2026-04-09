# Progress

## Completed

### Phase 1.1: Parser
- [x] Core model types (`pkg/model/`)
- [x] Markdown parser using goldmark (`pkg/parser/`)
- [x] Tests: 6 passing

### Phase 1.2: Serializer
- [x] Flow-to-markdown serializer (`pkg/serializer/`)
- [x] Round-trip tests (parse → serialize → parse): 3 passing

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

**Phase 1 milestone reached**: `flow run` and `flow validate` work end-to-end.

## Next

### Phase 2: Static visualization
- `flow viz` command outputting Mermaid diagram from parsed flow
