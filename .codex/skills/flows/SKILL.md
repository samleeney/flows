---
name: flows
description: "Use when the user asks to write, edit, run, validate, visualize, debug, or explain Flows markdown agent-flow documents. Covers notebook/org-mode style flow files that combine LLM prompt agents with deterministic code blocks, explicit inputs/outputs, start conditions, feedback loops, exhaustion routes, prompt executors, browser visualization, per-agent goal cards, and the distinction between goal cards and programmatic validation loops."
---

# Flows

Flows is a markdown based agent-flow writing system. A flow reads like a lightweight notebook or org-mode document: prose defines fuzzy agent work, fenced code blocks define deterministic programmatic work, and YAML metadata connects each block through named inputs, outputs, start rules, and loops.

Use this skill whenever the user wants to create, modify, run, inspect, visualize, or explain a `.md` flow.

## Locate the Tool

- Prefer the current repository when `./flow` exists.
- The known local project path is `/home/sam/personal_projects/flows`.
- If unsure about a command or schema detail, inspect local help or source before inventing syntax.

Useful commands:

```bash
./flow validate examples/jax_optimization_loop.md
./flow run examples/jax_optimization_loop.md --dry-run
./flow run examples/jax_optimization_loop.md -f --input code=@examples/inputs/slow_jax.py --input target_ms=5
./flow chart examples/jax_optimization_loop.md
./flow viz examples/jax_optimization_loop.md
```

By default, `./flow run <file>` prints a browser link for the run and starts it in the background. With `-f`, it prints the link and tails execution live.

## Authoring Model

Each flow file has:

1. YAML frontmatter with `name`, optional `description`, optional `external_inputs`, and optional `defaults`.
2. Top-level `##` sections. Each section is one block in the flow graph.
3. A first fenced `yaml` config block inside each section.
4. Either prompt markdown after the config, or a single executable fenced code block after the config.

Flows use explicit dataflow. A downstream block receives only its declared
`inputs`, each resolved from an external input or another block's latest raw
text output. Chat transcripts, hidden reasoning, and previous agent tool traces
are not passed automatically. If a later agent needs context, make the upstream
agent emit a handoff summary, rationale, structured result, or fenced artifact,
then pass that output as a named input.

Goal cards and validation loops are different concepts:

- If the user says "set up a goal", "the goal of this agent is...", or "give this agent a goal", attach a fenced `goal` block to that specific agent section. It becomes a special goal card above that agent in the chart, linked by a small `<->` association line. It is not a separate executable block.
- If the user asks for a loop of several agents followed by a programmatic check, create ordinary agent blocks plus a deterministic code block such as `benchmark` or `validate`. The code block is a normal executable block and can drive loop edges with `start` conditions like `when: benchmark`, `contains: too_slow`, and `max_runs`.
- These can coexist: an individual agent may have a visible goal card while a later code block performs the real pass/fail check for the loop. Do not replace deterministic checks with goal `validation` text.

Prompt block:

~~~markdown
## improve_speed

```yaml
inputs:
  code:
    from: external
start:
  - always: {max_runs: 1}
prompt_executor: codex_cli
model: gpt-5.3-codex-spark
```

Rewrite the input code to reduce runtime. Return only the improved code.
~~~

Programmatic block:

~~~markdown
## benchmark

```yaml
inputs:
  code:
    from: improve_speed
start:
  - when: improve_speed
```

```python
import time

start = time.perf_counter()
exec(inputs["code"], {})
elapsed = time.perf_counter() - start

output = {"elapsed": elapsed, "passed": elapsed < 1.0}
```
~~~

## Core Rules

- Use prompt blocks for fuzzy judgement, code rewriting, summaries, reviews, planning, and natural language transformation.
- Use programmatic blocks for validation, tests, benchmarks, parsing, scoring, file operations, deterministic transforms, and loop conditions.
- Make data movement explicit with `inputs`.
- Use `external_inputs` for values supplied by the caller.
- Use `--input name=value` for short values and `--input name=@path/to/file` for file contents.
- Use list-form `start` entries, such as `- always: {max_runs: 1}` and `- when: benchmark`.
- Use `contains: some_text` plus `max_runs` to express feedback loops.
- Use `on_exhaustion: stop` for the default error behavior, `on_exhaustion: continue` to mark an exhausted route handled, or `on_exhaustion: <agent_name>` to fire an exhaustion-handler agent once.
- Allow route-handler agents to omit ordinary `start` conditions when they are only reached through an `on_exhaustion` route.
- Prefer `codex_cli` with a quick Spark model for examples unless the user asks for another executor.
- Use `codex_cli_write` only for prompt nodes that should edit files in the repository.
- Use API executors only when the flow is intentionally meant to call an API and the required keys are available.
- Do not assume the interactive Codex `/goal` slash command works in `codex exec` headless mode. Headless goal-style execution should be modeled by explicit prompts or implemented with persistent Codex thread IDs and `codex exec resume <thread_id>`.
- Use an adjacent fenced `goal` block only for a goal attached to one agent card. Put it immediately after that agent's YAML config and before the normal prompt. Supported fields include `objective`, `validation`, `max_turns`, `token_budget`, and `on_exhaustion`.

Read `references/examples.md` when creating nontrivial flows, explaining state
handoff, using exhaustion routes, designing goal-style blocks, or debugging
loops and execution order.
