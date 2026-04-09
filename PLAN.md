# Agent Flow — Project Plan

## What it is

A declarative agent orchestration tool. Users define flows of LLM agents in a
single markdown file, execute them from a CLI, and optionally edit them visually
in a browser.

---

## The spec format

A single `.md` file defines an entire flow:

```
┌─────────────────────────────────────────┐
│  flow.md                                │
│                                         │
│  --- (frontmatter) ---                  │
│  Flow name, description, external inputs│
│                                         │
│  ## agent_name                          │
│  ```yaml``` → inputs, start, position   │
│  ```python/bash``` → function node      │
│  or plain text → LLM prompt             │
│                                         │
│  ## agent_name                          │
│  ...                                    │
└─────────────────────────────────────────┘
```

### Example flow file

```markdown
---
name: Code Review Flow
description: Automated review-fix loop with merge
external_inputs:
  - code
  - guidelines
---

## reviewer

~~~yaml
position: [0, 0]
inputs:
  code: { from: fixer, fallback: external }
  guidelines: { from: external }
start:
  - always: { max_runs: 1 }
  - when: fixer
    max_runs: 5
~~~

Review the provided code against the guidelines.

Focus on:
- Correctness
- Security vulnerabilities
- Style violations

Your first line of output MUST be either `needs_changes` or `approved`.

## fixer

~~~yaml
position: [1, 1]
inputs:
  code: { from: external }
  feedback: { from: reviewer }
start:
  - when: reviewer
    contains: "needs_changes"
~~~

You are given code and review feedback. Fix every issue raised.

Return the complete corrected code. Do not explain your changes.

## merger

~~~yaml
position: [2, 0]
inputs:
  code: { from: fixer, fallback: external }
start:
  - when: reviewer
    contains: "approved"
~~~

Merge this code. Summarise what was changed during review.
```

---

## Agent anatomy

Every agent has the same structure:

```
┌─────────────────────────────────┐
│  ## agent_name                  │
│                                 │
│  ┌── yaml config ────────────┐  │
│  │ position: [x, y]         │  │
│  │ inputs:                   │  │
│  │   data: { from: agent_a } │  │
│  │ start:                    │  │
│  │   - when: agent_a         │  │
│  │     contains: "ready"     │  │
│  │     max_runs: 5           │  │
│  └───────────────────────────┘  │
│                                 │
│  Content: either a prompt       │
│  (markdown text) or a code      │
│  block (python, bash, etc.)     │
└─────────────────────────────────┘
```

### Two types of node

- **LLM Agent Node**: Content is markdown text. Sent to an LLM as a prompt.
- **Function Node**: Content is a code block (python, bash, etc.). Executed locally.

```
  LLM Agent Node              Function Node
┌──────────────────┐     ┌──────────────────┐
│ yaml config      │     │ yaml config      │
│                  │     │                  │
│ Review this code │     │ ```python        │
│ for security     │     │ output = x + y   │
│ vulnerabilities..│     │ ```              │
│ (sent to LLM)   │     │ (executed locally)│
└──────────────────┘     └──────────────────┘
```

### Inputs

Each agent declares named inputs. Each input specifies where its data comes from:

- `{ from: agent_name }` — output of another agent
- `{ from: external }` — provided at flow start
- `{ from: agent_name, fallback: external }` — use agent output if available, otherwise external

The `|` / fallback mechanism handles first-iteration vs subsequent-iteration in loops:
on the first run the upstream agent hasn't produced output yet, so the fallback
(typically external) is used.

### Start conditions

Each agent declares when it should run:

- `always: { max_runs: N }` — run unconditionally, up to N times
- `when: agent_name` — run when the named agent has produced output
- `when: agent_name, contains: "text"` — run when output contains a string
- `when: [agent_a, agent_b]` — run when ALL named agents have produced output
- `max_runs: N` — cap on how many times this agent can fire per flow execution

An agent fires when its start condition is met AND all its inputs are available.

---

## Control flow

No separate flow graph. Control flow emerges from agent definitions:

```
    ┌──────────┐
    │ reviewer │ ← start: always (first run)
    └────┬─────┘   start: when fixer (max 5)
         │
    output text
         │
    ┌────┴─────────────────┐
    │                      │
    ▼                      ▼
contains              contains
"needs_changes"       "approved"
    │                      │
    ▼                      ▼
┌────────┐          ┌─────────┐
│ fixer  │          │ merger  │
└────┬───┘          └─────────┘
     │
     │ loops back
     └──────────→ reviewer (up to 5 times)
```

### Control flow patterns

| Pattern   | How it's expressed                                               |
|-----------|------------------------------------------------------------------|
| Sequence  | B's start condition references A                                 |
| Branch    | B and C both reference A with different conditions               |
| Join      | D's start condition requires both B AND C                        |
| Loop      | Mutual references with max_runs for termination                  |
| Parallel  | B and C both depend only on A, no mutual dependency              |

### Loop handling

Loops are mutual references with termination conditions:

```
     ┌──────────────────────────────┐
     │          Loop (max 5)        │
     │                              │
     │  ┌──────────┐  ┌─────────┐  │
     │  │ reviewer ├─→│  fixer  │  │
     │  │          │←─┤         │  │
     │  └──────────┘  └─────────┘  │
     │                              │
     └──────────────────────────────┘
                   │
          exit: "approved" OR 5 runs
                   │
                   ▼
             ┌──────────┐
             │  merger   │
             └──────────┘
```

Loop termination: whichever comes first of the exit condition being met or
max_runs being hit. When max_runs is hit without the exit condition, the flow
needs a policy (stop, continue, escalate). This should be configurable.

---

## Runtime execution model

```
flow run flow.md --input code=./src

    ┌─────────────────────────────────┐
    │           Runtime Loop          │
    │                                 │
    │  1. Seed external inputs        │
    │  2. Find agents whose:          │
    │     - start condition is met    │
    │     - inputs are available      │
    │  3. Run them (parallel if safe) │
    │  4. Register outputs            │
    │  5. Repeat until nothing fires  │
    └─────────────────────────────────┘
```

This is a dataflow execution model — the same idea behind Make, spreadsheets,
and hardware description languages. The runtime maintains a per-agent invocation
counter, reset at flow start, to enforce max_runs.

### Code block execution via plugins

The Go binary dispatches code blocks to language-specific executors:

```
┌─────────────────────────────────────────────┐
│  flow (Go binary)                      │
│                                             │
│  ┌─────────────┐                            │
│  │ prompt node  │ → LLM API (HTTP)          │
│  ├─────────────┤                            │
│  │ bash node    │ → sh -c (built-in)        │
│  ├─────────────┤                            │
│  │ python node  │ → python3 subprocess      │
│  ├─────────────┤                            │
│  │ node.js node │ → node subprocess         │
│  ├─────────────┤                            │
│  │ R node       │ → Rscript subprocess      │
│  └─────────────┘                            │
│                                             │
│  Contract: inputs as JSON on stdin          │
│            output as JSON on stdout         │
└─────────────────────────────────────────────┘
```

Each executor follows the same contract:
1. Receive input variables as JSON on stdin
2. Execute the code block
3. Return output as JSON on stdout

Language support is pluggable. The core binary ships with LLM prompt execution
and bash. Other languages (Python, Node, R, etc.) are optional — only needed if
the flow contains code blocks in that language.

---

## Visual editor

```
flow chart flow.md → opens localhost:8420

┌─ Browser ────────────────────────────────────┐
│                                              │
│  ┌──────────┐       ┌─────────┐             │
│  │ reviewer ├──────→│  fixer  │             │
│  │          │←──────┤         │             │
│  └─────┬────┘       └─────────┘             │
│        │                                     │
│        │ "approved"                          │
│        ▼                                     │
│  ┌──────────┐                                │
│  │  merger  │    ← drag, drop, connect       │
│  └──────────┘                                │
│                                              │
└──────────────────────────────────────────────┘
         ↕ WebSocket (bi-directional)
┌──────────────────────────────────────────────┐
│  Go binary                                   │
│  - serves React app (embedded static files)  │
│  - watches flow.md for external edits        │
│  - writes flow.md on visual changes          │
└──────────────────────────────────────────────┘
```

### Bi-directional sync

- Edit in the browser → updates the `.md` file on disk
- Edit the `.md` file in a text editor → updates the browser via WebSocket

### Technology

- **Frontend**: React Flow (purpose-built for node-based flow editors). Provides
  drag-and-drop, connection drawing, grid snapping, zoom/pan out of the box.
- **Backend**: Go HTTP/WebSocket server. Serves the React app as embedded static
  files via `embed.FS`. No Node.js required at runtime.
- **Layout**: Grid-based. Positions stored as grid coordinates in each agent's
  YAML config. Auto-layout via Dagre algorithm when positions are not specified.

### Editor capabilities

- Drag agent blocks on a snapped grid
- Draw connections between agent outputs and inputs
- Click a node to edit its config and prompt
- Add new agent nodes
- Delete nodes and connections
- All changes round-trip to the markdown file

---

## What ships

```
One binary: flow

  flow run flow.md              # execute a flow
  flow chart flow.md             # open visual editor
  flow validate flow.md         # check for errors
  flow viz flow.md              # print mermaid/dot diagram
```

Single binary, no runtime dependencies. Built in Go with the React editor
compiled into the binary. Language plugins only needed if the flow uses code
blocks in that language.

---

## Build order

```
1. Parser           markdown → in-memory representation
2. Serializer       in-memory → markdown (needed for editor writes)
3. Runtime          execute flows, manage agent lifecycle
4. Editor backend   HTTP/WebSocket server
5. Editor frontend  React Flow UI
```

---

## Open design questions

These were identified during design but not fully resolved. The implementer
should make decisions on these early.

### Agent output format

Agent output is raw text (LLM output is text). Downstream start conditions need
to inspect that text (e.g. `contains: "approved"`). Options for how to handle
this, in order of complexity:

1. **String matching / regex on raw output** — simple, fragile
2. **Require agents to emit structured markers** — e.g. first line must be
   `STATUS: approved` or `STATUS: needs_changes`
3. **Output parser per agent** — agent config includes a parser that extracts
   typed fields from raw text
4. **Small router LLM call** — a cheap model classifies the output

Recommendation: start with option 2 (structured markers) as it's simple and
explicit. Can add parsers later if needed.

### Failure handling

What happens when an agent errors, times out, or a loop hits max_runs without
the exit condition being met? Options:

- **Stop the whole flow** and report the error
- **Skip downstream agents** that depend on the failed agent
- **Retry** the failed agent (with configurable max retries)
- **Fire a dedicated error-handler agent**

This should be configurable per-agent and per-flow, with sensible defaults
(e.g. stop on error, configurable timeout).

### Input merge semantics

When a node has two incoming edges both providing the same named input (e.g. two
upstream agents both produce `code`), the runtime needs a rule:

- Last writer wins
- Error (require disambiguation)
- Explicit merge/pick node

Recommendation: error by default, require the user to disambiguate. Silent
last-writer-wins leads to subtle bugs.

### Context vs data

Worth distinguishing between:

- **Data**: specific to this run (the code, the review comments, agent outputs)
- **Context**: ambient configuration (which LLM model to use, temperature,
  system prompt template, API keys)

Context could be inherited/overridden hierarchically (flow-level defaults,
per-agent overrides) rather than wired through input ports. This keeps the data
flow graph clean. Example in frontmatter:

```yaml
---
name: Code Review Flow
defaults:
  model: claude-sonnet-4-20250514
  temperature: 0.3
---
```

With per-agent override:

```yaml
inputs: ...
start: ...
model: claude-opus-4-0-20250115
temperature: 0.7
```

### Parser implementation

Use a markdown parsing library (e.g. goldmark for Go) rather than hand-rolling
regex-based splitting. The parser needs to:

1. Extract YAML frontmatter (`---` to `---`)
2. Split on `## ` headings
3. For each section: extract first yaml code block as config, detect if a code
   block in another language follows (function node) or remaining text (prompt)

A library handles edge cases (nested code blocks, escaped characters) that
regex will miss.

### Scaling

The markdown single-file format works well for flows of 3-15 agents. For larger
flows:

- Support an `include` directive in frontmatter to compose from multiple files
- At hundreds+ of agents, flows are likely generated (not hand-authored), so the
  markdown format becomes an intermediate representation
- The parser and runtime scale fine to thousands of agents; the human authoring
  experience is what breaks

---

## Alternatives considered and rejected

These were evaluated during design. Documented here so the implementer
understands why certain paths were not taken.

### Custom DSL

A purpose-built language for defining flows. Rejected because: building a
parser, runtime, error reporting, and tooling from scratch is expensive. DSLs
tend to grow scope until they become bad general-purpose languages. Every user
has to learn new syntax.

### Pure YAML

All config, including prompts, in YAML. Rejected because: prompts are long,
formatted text. In YAML they'd be ugly multiline strings indented 4 levels deep,
or external file references (at which point you have multiple files anyway).
Markdown makes prompts first-class.

### Python with a graph API (LangGraph-style)

Define flows in Python using a constrained API that builds a graph. Rejected
for this project because: deployment friction (requires Python runtime), and the
target use case (CI, servers, GitHub Actions) favours a single binary. However,
LangGraph is good prior art — the execution model is similar.

### Pure Python

Write flows as imperative Python code. Rejected because: the flow structure
gets buried in code, hard to visualize/serialize/inspect, nothing prevents
spaghetti.

### Embedding equations/algorithms in the spec

Adding expression evaluation to the YAML config (e.g. `score > 0.8`,
arithmetic). Rejected because of scope creep — every workflow tool that adds
expressions eventually becomes a bad programming language (see: GitHub Actions
`${{ }}` expressions). Instead, computation lives in function nodes (code
blocks) where a real language handles it properly.

### Shared blackboard state model

All agents read/write a single shared state dict (the LangGraph approach).
Rejected in favour of explicit named inputs because: implicit coupling makes it
impossible to tell from the graph which agent needs what, naming collisions
occur, and parallel execution is unsafe.

### Message passing / event-based data flow

Agents emit typed messages, downstream agents subscribe by message type.
Rejected because: flow structure becomes implicit in subscriptions rather than
visible in the graph, harder to reason about, and debugging "why didn't this
agent fire?" is painful.

---

## Key design decisions

1. **Markdown as the spec format** — prompts are the most important part of each
   agent and they're long, formatted text. Markdown is their natural home.

2. **Self-describing agents** — each agent declares its own inputs, start
   conditions, and content. The flow graph is implied, not separately defined.

3. **Go for the binary** — single binary deployment matters for CI/GitHub
   Actions/server use cases. No runtime dependencies.

4. **Pluggable language executors** — the core binary doesn't need to know every
   language. Code blocks dispatch to subprocesses via a simple JSON stdin/stdout
   contract.

5. **Grid-based visual editor** — React Flow frontend, embedded in the binary,
   bi-directional sync with the markdown file.

6. **Reactive/dataflow execution** — agents fire when their conditions are met.
   No imperative control flow. Loops emerge from mutual references with
   termination caps.

---

## Mental models and prior art

These analogies help explain the design intent:

- **Make**: targets declare dependencies and recipes. `make` resolves the graph
  and executes in order. This project is the same idea — agents declare
  dependencies (start conditions) and the runtime resolves execution order.

- **Emacs Org Mode**: a single file mixes prose and executable code blocks in
  multiple languages, and they work together. The markdown flow format follows
  the same philosophy — prompts (prose) and code blocks coexist in one document.

- **Spreadsheets**: cells reference other cells, and the engine recalculates
  when dependencies change. The reactive execution model here works the same
  way — agents fire when their referenced inputs become available.

- **Hardware description languages**: describe components and their wiring
  declaratively; a simulator resolves timing and execution. Similar to how
  agents describe their connections and the runtime resolves execution order.
