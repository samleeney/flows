---
name: JAX Goal And Benchmark Loop
description: Show a per-agent goal card plus a separate JAX benchmark loop gate
external_inputs:
    - code
    - target_ms
defaults:
    prompt_executor: codex_cli
    model: gpt-5.3-codex-spark
    temperature: 0.2
---

## speed_optimizer

```yaml
position: [8, 5]
inputs:
    code: {from: benchmark, fallback: external}
    target_ms: {from: external}
start:
    - always: {max_runs: 1}
    - when: benchmark
      contains: too_slow
      max_runs: 3
      on_exhaustion: summarize_failure
```

```goal
objective: Reduce the runtime of the JAX loss function below target_ms.
validation:
    - Return exactly one fenced python block.
    - Preserve the public function named loss.
    - Preserve the numerical meaning of the result.
    - Mention benchmark uncertainty only in a short note after the code.
max_turns: 3
on_exhaustion: summarize_failure
```

Use the provided `code` input. If it contains benchmark feedback, extract the
latest fenced Python block and use that as the starting point.

Optimize for JAX runtime. Prefer `jax.jit`, `jax.vmap`, `jax.lax.scan`, and
fused array operations. Remove Python loops over array rows when they block JAX
compilation.

Return one complete fenced `python` block containing the revised code, followed
by at most two sentences of handoff notes.

## memory_optimizer

```yaml
position: [16, 6]
inputs:
    code: {from: speed_optimizer}
    target_ms: {from: external}
start:
    - when: speed_optimizer
```

Improve peak memory use without undoing the speed changes.

Keep the same public function names and return values. Return only one complete
fenced `python` block.

## benchmark

```yaml
position: [25, 6]
inputs:
    code: {from: memory_optimizer}
    target_ms: {from: external}
start:
    - when: memory_optimizer
```

```python
import re
import time

import jax
import jax.numpy as jnp


def latest_python_block(text):
    blocks = re.findall(r"```(?:python)?\n(.*?)```", text, flags=re.S)
    return blocks[-1].strip() if blocks else text.strip()


candidate = latest_python_block(code)
target = float(target_ms)

namespace = {}
exec(candidate, namespace)
loss_fn = namespace["loss"]
x = jnp.ones((256, 128), dtype=jnp.float32)
w = jnp.linspace(0.1, 1.0, 128, dtype=jnp.float32)

for _ in range(2):
    loss_fn(x, w).block_until_ready()

started = time.perf_counter()
loss_fn(x, w).block_until_ready()
measured_ms = (time.perf_counter() - started) * 1000.0

status = "fast_enough" if measured_ms <= target else "too_slow"
fence = "`" * 3

output = f"""{status}
measured_ms: {measured_ms:.2f}
target_ms: {target:.2f}
benchmark: actual_jax_timing

{fence}python
{candidate}
{fence}"""
```

## summarize_failure

```yaml
position: [41, 7]
inputs:
    code: {from: external}
    latest_benchmark: {from: benchmark}
```

The speed optimizer exhausted its retry budget.

Summarize:
- The latest measured runtime and target.
- The most likely remaining bottleneck.
- One safe next optimization attempt.
