---
name: flows
description: "Use when the user asks to write, edit, run, validate, visualize, debug, or explain Flows markdown agent-flow documents. Covers notebook/org-mode style flow files that combine LLM prompt agents with deterministic code blocks, explicit inputs/outputs, start conditions, feedback loops, prompt executors, browser visualization, and real execution examples such as the JAX optimization loop."
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
- Prefer `codex_cli` with a quick Spark model for examples unless the user asks for another executor.
- Use `codex_cli_write` only for prompt nodes that should edit files in the repository.
- Use API executors only when the flow is intentionally meant to call an API and the required keys are available.

For detailed patterns, examples, and troubleshooting, read `references/examples.md`.
