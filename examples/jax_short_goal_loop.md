---
name: Short JAX Speed Goal Loop
description: Minimal speed optimizer goal card with a benchmark feedback loop
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
      on_exhaustion: continue
```

```goal
objective: Make the JAX loss function run below target_ms.
validation:
    - Return exactly one fenced python block.
    - Preserve the public function named loss.
    - Preserve the numerical meaning of the result.
max_turns: 3
on_exhaustion: continue
```

Use the `code` input. If it contains benchmark feedback, extract the latest
fenced Python block and improve that version.

Optimize for speed with JAX-native transforms such as `jax.jit`, `jax.vmap`,
and fused array operations. Avoid changing the public `loss(x, w)` interface.

Return only one complete fenced `python` block.

## benchmark

```yaml
position: [16, 6]
inputs:
    code: {from: speed_optimizer}
    target_ms: {from: external}
start:
    - when: speed_optimizer
```

```python
import re
import time

import jax.numpy as jnp


def latest_python_block(text):
    blocks = re.findall(r"```(?:python)?\n(.*?)```", text, flags=re.S)
    return blocks[-1].strip() if blocks else text.strip()


candidate = latest_python_block(code)
target = float(target_ms)

namespace = {}
exec(candidate, namespace)
loss_fn = namespace["loss"]

x = jnp.ones((128, 64), dtype=jnp.float32)
w = jnp.linspace(0.1, 1.0, 64, dtype=jnp.float32)

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

{fence}python
{candidate}
{fence}"""
```
