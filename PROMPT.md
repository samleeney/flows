You are implementing the "flow" tool as described in PRD.md. Read the PRD carefully at the start of each iteration.

## Working directory

/home/sam/personal_projects/flows

## Build order (from PRD Section 9)

Follow this order strictly. Do NOT skip ahead:

1. Parser - Go module, parse .md flow files into in-memory structs. Use goldmark for markdown parsing. Include YAML frontmatter extraction, section splitting, config block extraction, node type detection.
2. Serializer - write structs back to .md format (round-trip preserving).
3. Validation - reference checking, cycle detection, max_runs enforcement on cycles, clear error messages.
4. Runtime - dataflow scheduling loop, LLM prompt execution, bash code block execution, parallel independent agents, invocation counting, external input injection.
5. Static visualization - flow viz outputs Mermaid diagram.
6. Editor backend - Go HTTP/WebSocket server.
7. Editor frontend - React Flow UI with drag-and-drop.
8. Bi-directional sync - file watcher + serializer.

## Per-iteration instructions

Each iteration:

1. Read PRD.md to understand the full requirements
2. Check PROGRESS.md (create it if it doesn't exist) to see what has been done
3. Pick the next incomplete item from the build order
4. Implement it with tests
5. Make sure all existing tests pass (go test ./...)
6. Commit your work with a clear message
7. Update PROGRESS.md marking what you completed and what is next

## Quality standards

- Write idiomatic Go
- Include unit tests for each package
- Use the example flow from PRD Section 2.4 as a test fixture
- Handle errors properly, no panics
- Keep packages well-separated: cmd/, pkg/parser/, pkg/serializer/, pkg/runtime/, pkg/validator/, internal/

## Completion

When ALL phases (1-8) are complete and all tests pass, output: IMPLEMENTATION COMPLETE
