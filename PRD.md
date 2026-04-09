# Flow — Product Requirements Document

## 1. Overview

### 1.1 Problem statement

LLM agents can accomplish tasks individually, but complex workflows — such as a
code review pipeline where a reviewer agent must run before a fixer agent, which
loops back for re-review, and a merger agent only fires on approval — require
programmatic orchestration. Existing solutions are either too heavyweight
(LangGraph, AutoGen), require a Python runtime (bad for CI/server deployment),
or bury the flow structure in imperative code where it can't be visualized or
inspected.

### 1.2 Product summary

Flow is a declarative agent orchestration tool. Users define workflows of LLM
agents in a single markdown file, execute them from a CLI, and optionally
manipulate them visually in a browser-based flowchart editor. It ships as a
single Go binary with zero runtime dependencies.

### 1.3 Target users

- **DevOps / platform engineers** integrating LLM agents into CI pipelines and
  GitHub Actions
- **AI engineers** building multi-agent systems that need structured
  orchestration beyond simple chaining
- **Technical team leads** who need to visualize, review, and reason about agent
  workflows

### 1.4 Design principles

- **Prompts are first-class citizens** — they're the most important part of each
  agent and should not be strings buried in config
- **Self-describing agents** — each agent declares everything about itself
  (inputs, conditions, content); the flow graph is implied, not separately
  defined
- **No new programming language** — computation lives in real languages via code
  blocks; the spec is pure wiring and configuration
- **Zero-friction deployment** — a single binary, no runtime dependencies, drop
  into CI and run

---

## 2. Spec format

### 2.1 File structure

A single `.md` file defines an entire flow. The file uses standard markdown
constructs: YAML frontmatter, `##` headings, fenced code blocks, and prose.

```
┌─────────────────────────────────────────┐
│  flow.md                                │
│                                         │
│  --- (YAML frontmatter) ---             │
│  Flow name, description, external inputs│
│  Default model/config                   │
│                                         │
│  ## agent_name                          │
│  ```yaml``` → inputs, start, position   │
│  prose text → LLM prompt                │
│  OR ```python/bash``` → function node   │
│                                         │
│  ## agent_name                          │
│  ...                                    │
└─────────────────────────────────────────┘
```

### 2.2 Frontmatter

The YAML frontmatter block defines flow-level metadata:

```yaml
---
name: Code Review Flow
description: Automated review-fix loop with merge
external_inputs:
  - code
  - guidelines
defaults:
  model: claude-sonnet-4-20250514
  temperature: 0.3
---
```

| Field              | Required | Description                                    |
|--------------------|----------|------------------------------------------------|
| `name`             | Yes      | Human-readable flow name                       |
| `description`      | No       | What the flow does                             |
| `external_inputs`  | Yes      | Named inputs provided at flow start            |
| `defaults`         | No       | Default LLM config inherited by all agents     |

### 2.3 Agent definition

Each `##` heading defines an agent. The heading text is the agent's unique name
(must be a valid identifier: lowercase alphanumeric + underscores).

Under each heading:

1. **First fenced `yaml` code block** — agent configuration (inputs, start
   conditions, grid position, optional config overrides)
2. **Content after the YAML block** — either:
   - **Markdown prose** → LLM prompt (sent to a model)
   - **Fenced code block in another language** (python, bash, etc.) → function
     node (executed locally)

The runtime distinguishes node type by what follows the YAML config block.

### 2.4 Complete example

```markdown
---
name: Code Review Flow
description: Automated review-fix loop with merge
external_inputs:
  - code
  - guidelines
defaults:
  model: claude-sonnet-4-20250514
  temperature: 0.3
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

## 3. Agent specification

### 3.1 Inputs

Each agent declares named inputs. Each input specifies its data source:

| Syntax                                       | Meaning                                              |
|----------------------------------------------|------------------------------------------------------|
| `{ from: agent_name }`                       | Output of another agent                              |
| `{ from: external }`                         | Provided at flow start via CLI                       |
| `{ from: agent_name, fallback: external }`   | Agent output if available, else external             |

The fallback mechanism handles first-iteration vs subsequent-iteration in loops.
On the first run, the upstream agent hasn't produced output yet, so the fallback
is used.

**Input merge conflict**: if two upstream agents could both provide the same
named input, the runtime MUST error at validation time rather than silently
picking one. The user should disambiguate with explicit input wiring.

### 3.2 Start conditions

Each agent declares one or more start conditions. The agent fires when ANY
condition is met (OR semantics across the list) AND all its declared inputs are
available.

| Condition                                     | Meaning                                         |
|-----------------------------------------------|--------------------------------------------------|
| `always: { max_runs: N }`                     | Run unconditionally, up to N times               |
| `when: agent_name`                            | Run when the named agent has produced new output  |
| `when: agent_name, contains: "text"`          | Run when output contains the given string         |
| `when: [agent_a, agent_b]`                    | Run when ALL named agents have produced output    |
| `max_runs: N`                                 | Cap on total invocations per flow execution       |

### 3.3 Node types

**LLM Agent Node**: content is markdown prose. The runtime sends it to an LLM
as a prompt, with declared inputs injected as context. Output is the raw text
response from the model.

**Function Node**: content is a fenced code block in a supported language. The
runtime executes it locally. Inputs are provided as JSON on stdin. Output is
read from stdout as JSON.

### 3.4 Agent output

All agent output is raw text. Downstream start conditions that need to inspect
output (e.g. `contains: "approved"`) operate on this text via string matching.

Agents that need to communicate structured decisions to downstream conditions
should emit a structured marker as their first line of output (e.g.
`needs_changes` or `approved`). The prompt should instruct the agent to do this.

### 3.5 Configuration overrides

Agents inherit flow-level defaults from the frontmatter. Per-agent overrides
are specified in the agent's YAML config block:

```yaml
inputs: ...
start: ...
model: claude-opus-4-0-20250115
temperature: 0.7
```

This separates ambient configuration (model, temperature, API settings) from
data flow (inputs/outputs), keeping the flow graph clean.

### 3.6 Grid position

Each agent optionally declares its position on the visual editor grid:

```yaml
position: [x, y]
```

These are grid coordinates (integers), not pixel values. The visual editor maps
them to screen positions. When not specified, the editor auto-layouts using a
graph layout algorithm (Dagre).

---

## 4. Control flow

### 4.1 Core principle

There is no separate flow graph definition. Control flow emerges entirely from
agent definitions. The runtime collects all agents, resolves the dependency
graph from their `inputs` and `start` fields, and executes.

### 4.2 Control flow patterns

| Pattern    | How it's expressed                                               |
|------------|------------------------------------------------------------------|
| Sequence   | B's start condition references A                                 |
| Branch     | B and C both reference A with different conditions               |
| Join       | D's start condition requires both B AND C (`when: [B, C]`)      |
| Loop       | Mutual references between agents with `max_runs` for termination |
| Parallel   | B and C both depend only on A, no mutual dependency              |

### 4.3 Loop handling

Loops emerge from mutual references. For example, a reviewer-fixer loop:

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

Loop termination occurs when EITHER the exit condition is met (e.g. reviewer
says "approved") OR the `max_runs` cap is hit — whichever comes first.

### 4.4 Loop exhaustion policy

When a loop hits `max_runs` without the exit condition being met, the runtime
needs a configurable policy:

- `stop` — halt the entire flow and report an error (default)
- `continue` — proceed to downstream agents as if the exit condition was met
- `escalate` — fire a designated error-handler agent

This is configurable per-agent in the YAML config block:

```yaml
start:
  - when: fixer
    max_runs: 5
    on_exhaustion: continue
```

---

## 5. Runtime

### 5.1 Execution model

The runtime uses a reactive dataflow execution model:

```
flow run flow.md --input code=./src --input guidelines=./STYLE.md

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

The runtime maintains a per-agent invocation counter (reset at flow start) to
enforce `max_runs`. Independent agents (no mutual data dependencies) execute in
parallel.

### 5.2 Validation

Before execution, the runtime MUST validate the flow:

- All `from` references point to agents that exist in the flow
- All external inputs referenced by agents are declared in frontmatter
- No unconnected inputs (every declared input has a valid source)
- No ambiguous inputs (two agents providing the same named input to a downstream
  agent without explicit disambiguation)
- Cycles are detected and must include at least one `max_runs` cap to prevent
  infinite loops
- Agent names are valid identifiers (lowercase alphanumeric + underscores)

Validation errors are reported with the agent name and specific field that
failed, so the user can locate the problem in the markdown file.

`flow validate flow.md` runs validation without executing.

### 5.3 Code block execution

The Go binary dispatches code blocks to language-specific executors:

```
┌──────────────────────────────────────────┐
│  flow (Go binary)                        │
│                                          │
│  ┌─────────────┐                         │
│  │ prompt node  │ → LLM API (HTTP)       │
│  ├─────────────┤                         │
│  │ bash node    │ → sh -c (built-in)     │
│  ├─────────────┤                         │
│  │ python node  │ → python3 subprocess   │
│  ├─────────────┤                         │
│  │ node.js node │ → node subprocess      │
│  └─────────────┘                         │
│                                          │
│  Contract: inputs as JSON on stdin       │
│            output as JSON on stdout      │
└──────────────────────────────────────────┘
```

Each executor follows the same contract:
1. Receive input variables as JSON on stdin
2. Execute the code block
3. Return output as JSON on stdout
4. Non-zero exit code = agent failure

Language support is pluggable. The core binary ships with LLM prompt execution
and bash. Other languages are optional — only needed if the flow contains code
blocks in that language. The runtime should error clearly if a flow uses a
language whose executor is not available.

### 5.4 Failure handling

When an agent fails (error, timeout, non-zero exit):

- **Default**: stop the entire flow and report which agent failed, with its
  stderr/error output
- **Per-agent override**: configurable via `on_error` in the YAML config block:
  - `stop` — halt the flow (default)
  - `retry: N` — retry up to N times before stopping
  - `skip` — skip this agent and any downstream agents that depend solely on it
  - `escalate: agent_name` — fire a designated error-handler agent

---

## 6. Visual editor

### 6.1 Overview

`flow chart flow.md` starts a local web server and opens a browser-based
flowchart editor where users can visually manipulate the flow.

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

### 6.2 Bi-directional sync

- **Browser → file**: user drags a block, draws a connection, edits config, or
  adds/deletes a node → WebSocket message → Go backend updates the `.md` file
  on disk
- **File → browser**: user edits the `.md` file in a text editor → Go backend
  detects change via file watcher → pushes new state to browser via WebSocket

Both directions must preserve content that the other side doesn't understand
(e.g. the visual editor must not strip markdown formatting from prompts).

### 6.3 Technology

- **Frontend**: React Flow — purpose-built library for node-based flow editors.
  Provides drag-and-drop, connection drawing, grid snapping, and zoom/pan.
- **Backend**: Go HTTP server + WebSocket. Serves the React app as embedded
  static files via Go's `embed.FS`. No Node.js required at runtime.
- **Layout**: Grid-based. Positions stored as integer grid coordinates in each
  agent's YAML config. Auto-layout via Dagre algorithm when positions are absent.

### 6.4 Editor capabilities

| Capability                  | Description                                          |
|-----------------------------|------------------------------------------------------|
| Drag blocks                 | Move agent nodes on a snapped grid                   |
| Draw connections            | Wire agent outputs to downstream inputs              |
| Edit config                 | Click a node to edit its YAML config and prompt       |
| Add nodes                   | Create new agent blocks                              |
| Delete nodes/connections    | Remove agents or connections                         |
| Auto-layout                 | Arrange nodes automatically when positions are absent |

All changes round-trip to the markdown file.

---

## 7. CLI interface

### 7.1 Commands

```
flow run <file>              Execute a flow
flow chart <file>            Open visual editor in browser
flow validate <file>         Validate a flow without executing
flow viz <file>              Output flow graph as Mermaid or DOT
```

### 7.2 `flow run` options

| Flag                        | Description                                      |
|-----------------------------|--------------------------------------------------|
| `--input name=value`        | Provide an external input (repeatable)           |
| `--input name=@filepath`    | Provide an external input from a file            |
| `--verbose`                 | Print agent execution details                    |
| `--dry-run`                 | Validate and show execution plan without running |
| `--output <dir>`            | Write each agent's output to a file in this dir  |

### 7.3 `flow chart` options

| Flag                        | Description                                      |
|-----------------------------|--------------------------------------------------|
| `--port <N>`                | Port for the editor server (default: 8420)       |
| `--no-open`                 | Don't auto-open the browser                      |

---

## 8. Distribution

### 8.1 Single binary

The tool ships as a single statically-compiled Go binary. The React editor
frontend is compiled to static files and embedded in the binary via Go's
`embed.FS`. No runtime dependencies.

### 8.2 Installation

```bash
# Direct download
curl -L https://github.com/samleeney/flows/releases/latest/download/flow-linux-amd64 \
  -o /usr/local/bin/flow && chmod +x /usr/local/bin/flow

# In a GitHub Actions workflow
- run: curl -L .../flow-linux-amd64 -o /usr/local/bin/flow && chmod +x /usr/local/bin/flow
- run: flow run my-flow.md --input code=./src
```

### 8.3 Language plugin dependencies

The core binary requires no external dependencies. Language-specific code block
execution requires the corresponding runtime on PATH:

| Code block language | Requires on PATH |
|---------------------|------------------|
| bash                | sh (built-in)   |
| python              | python3          |
| javascript / node   | node             |
| R                   | Rscript          |

The runtime errors clearly if a flow uses a language whose interpreter is not
available.

---

## 9. Build order and milestones

### Phase 1: Core (parser + runtime)

1. **Parser** — parse a `.md` flow file into in-memory representation
   - YAML frontmatter extraction
   - Section splitting on `##` headings
   - YAML config block extraction per agent
   - Node type detection (prompt vs function)
   - Use goldmark or similar Go markdown library
2. **Serializer** — write in-memory representation back to `.md` (needed for
   the visual editor; also enables `validate` to reformat)
3. **Validation** — check references, detect cycles, enforce `max_runs` on
   cycles, report clear errors
4. **Runtime** — execute a parsed flow
   - Dataflow scheduling loop
   - LLM prompt execution (HTTP calls to LLM APIs)
   - Bash code block execution
   - Parallel execution of independent agents
   - Invocation counting and `max_runs` enforcement
   - External input injection from CLI flags

**Milestone**: `flow run` and `flow validate` work end-to-end for a simple
flow with LLM agents and bash function nodes.

### Phase 2: Visualization

5. **Static visualization** — `flow viz` outputs Mermaid or DOT graph from a
   parsed flow

**Milestone**: `flow viz` produces a viewable diagram of any flow.

### Phase 3: Visual editor

6. **Editor backend** — Go HTTP/WebSocket server serving the parsed flow as
   JSON and accepting updates
7. **Editor frontend** — React Flow-based UI with drag-and-drop, connection
   drawing, grid snapping, node editing
8. **Bi-directional sync** — file watcher for external edits, serializer for
   visual edits back to markdown

**Milestone**: `flow chart` opens a functional visual editor that round-trips
to the markdown file.

### Phase 4: Polish and extensibility

9. **Additional language executors** — Python, Node.js, R
10. **Error handling and retry** — per-agent `on_error` and `on_exhaustion`
    policies
11. **Cross-compilation and release pipeline** — build for linux/mac/windows,
    amd64/arm64, GitHub Releases

---

## 10. Open design questions

These were identified during design but intentionally left unresolved. The
implementer should make decisions on these early.

### 10.1 Output inspection mechanism

Start conditions like `contains: "approved"` require inspecting agent output.
Current recommendation is simple string matching with structured markers (agent
prompts instruct the LLM to emit a status keyword as the first line). May need
to evolve to regex matching or output parsers if string matching proves too
fragile.

### 10.2 Scaling beyond single-file

For flows with 15+ agents, a single markdown file becomes unwieldy. An
`include` directive in the frontmatter could compose a flow from multiple files.
Design this when the need arises — don't build it speculatively.

### 10.3 LLM API configuration

The flow needs to know which LLM provider to call and how to authenticate. This
could be:
- Environment variables (e.g. `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`)
- A config file (`~/.flow/config.yaml`)
- Per-flow frontmatter

Recommendation: environment variables for API keys (standard practice), flow
frontmatter for model selection and parameters.

---

## 11. Alternatives considered and rejected

Documented so the implementer understands why certain paths were not taken.

| Alternative                        | Why rejected                                                |
|------------------------------------|-------------------------------------------------------------|
| Custom DSL                         | Expensive to build parser/tooling; DSLs grow into bad languages |
| Pure YAML (no markdown)            | Prompts are long formatted text; YAML makes them second-class |
| Python graph API (LangGraph-style) | Deployment friction; requires Python runtime               |
| Pure imperative Python             | Flow structure buried in code; can't visualize/inspect     |
| Expressions in the spec            | Scope creep; becomes a bad language (see GitHub Actions)   |
| Shared blackboard state            | Implicit coupling; unsafe for parallel execution           |
| Message passing / event-based      | Flow structure implicit in subscriptions; hard to debug    |
| Rust for the binary                | Slower development; LLM ecosystem thinner; Go suffices     |

---

## 12. Mental models and prior art

These analogies convey the design intent:

- **Make** — targets declare dependencies and recipes; `make` resolves the graph.
  Flow agents declare dependencies (start conditions); the runtime resolves
  execution order.
- **Emacs Org Mode** — a single file mixes prose and executable code blocks in
  multiple languages. The flow format follows the same philosophy.
- **Spreadsheets** — cells reference other cells; the engine recalculates when
  dependencies change. The reactive execution model works identically.
- **Hardware description languages** — describe components and wiring
  declaratively; a simulator resolves timing. Similar to how agents describe
  connections and the runtime resolves execution.
