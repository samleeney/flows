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
