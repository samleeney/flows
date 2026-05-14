# Flows

Flows is a markdown-based agent flow writing system. A flow reads like a small
notebook or Emacs org-mode document: front matter defines global settings, each
heading defines an agent or programmatic step, and fenced blocks define that
step's inputs, triggers, and executable code.

The useful idea is mixing fuzzy agent work with deterministic program steps.
Prompt agents can rewrite, review, or summarize; code blocks can benchmark,
parse, validate, or transform; loops connect them until a condition is met. Each
step has explicit inputs and outputs, so the same document can be executed,
tested, and visualized in the browser.

## JAX Optimization Example

The example flow is `examples/jax_optimization_loop.md`. It starts from
deliberately slow JAX code in `examples/inputs/slow_jax.py` and a runtime target:

```bash
./flow run examples/jax_optimization_loop.md -f --input code="$(cat examples/inputs/slow_jax.py)" --input target_ms=5
```

The flow is split into four blocks:

1. `speed_optimizer` is an agent prompt. It receives the original code, or later
   benchmark feedback, and rewrites the JAX for runtime.
2. `memory_optimizer` is another agent prompt. It receives the speed-optimized
   code and reduces unnecessary allocation.
3. `waste_reducer` is an agent prompt. It removes debug output, dead code, and
   verbosity while preserving the prior optimizations.
4. `benchmark` is a Python code block. It imports real `jax`, executes the
   candidate code, measures runtime, and emits either `fast_enough` or
   `too_slow`.

The loop is declared in markdown, not hidden in code. If `benchmark` outputs
`too_slow`, the result feeds back into `speed_optimizer`:

```yaml
start:
  - always: {max_runs: 1}
  - when: benchmark
    contains: too_slow
    max_runs: 3
```

That makes the document both readable and executable: fuzzy optimization steps
are linked to a programmatic benchmark, and the benchmark controls whether the
agent loop continues.

## Visualizing

Every flow can be opened in the browser:

```bash
./flow chart examples/jax_optimization_loop.md
```

The UI shows agent blocks, code blocks, local input/output blocks, generated
output links, and loop conditions. Clicking a block shows the inputs, triggers,
content, and routing details.
