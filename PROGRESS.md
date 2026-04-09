# Progress

## Completed

### Phase 1.1: Parser
- [x] Core model types (`pkg/model/model.go`, `pkg/model/yaml.go`)
  - Flow, Agent, Input, Condition, NodeType, StringOrList with custom YAML marshal
- [x] Markdown parser (`pkg/parser/parser.go`)
  - YAML frontmatter extraction
  - Section splitting on `##` headings via goldmark AST
  - YAML config block extraction per agent
  - Node type detection (prompt vs function)
  - Fallback input support
- [x] Tests: 6 passing (code review flow, function node, error cases)

## Next

### Phase 1.2: Serializer
- Write in-memory Flow back to markdown format
- Round-trip preserving (parse → serialize → parse should be equivalent)
