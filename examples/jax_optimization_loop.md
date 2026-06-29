---
name: JAX Optimization Loop
description: Iteratively optimize slow JAX code for speed, memory, and concision until it meets a runtime target
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
position: [8, 4]
inputs:
    code: {from: benchmark, fallback: external}
    target_ms: {from: external}
start:
    - always: {max_runs: 1}
    - when: benchmark
      contains: too_slow
      max_runs: 3
```

You are optimizing JAX code for minimum runtime.

If the `code` input contains benchmark feedback, extract the latest Python code
block from that feedback and use it as the starting point.

Goal:
- Make the code run under `target_ms`.
- Prefer `jax.jit`, `jax.vmap`, `jax.lax.scan`, and fused array operations.
- Remove Python loops over array rows when they block JAX compilation.
- Preserve the public function names and return values.

Return only one fenced `python` code block containing the complete revised code.

## memory_optimizer

```yaml
position: [16, 5]
inputs:
    code: {from: speed_optimizer}
    target_ms: {from: external}
start:
    - when: speed_optimizer
```

You are optimizing JAX code for minimum peak memory while keeping the speed
target in mind.

Goal:
- Avoid building large intermediate lists or stacked arrays.
- Prefer reductions that do not materialize unnecessary temporaries.
- Keep the same public function names and outputs.

Return only one fenced `python` code block containing the complete revised code.

## waste_reducer

```yaml
position: [25, 6]
inputs:
    code: {from: memory_optimizer}
    target_ms: {from: external}
start:
    - when: memory_optimizer
```

You are removing waste and making the optimized JAX code concise.

Goal:
- Remove debug prints, unused imports, dead variables, and noisy comments.
- Keep the implementation short, readable, and faithful to the original API.
- Do not undo speed or memory improvements.

Return only one fenced `python` code block containing the complete revised code.

## benchmark

```yaml
position: [33, 6]
inputs:
    code: {from: waste_reducer}
    target_ms: {from: external}
start:
    - when: waste_reducer
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

start = time.perf_counter()
loss_fn(x, w).block_until_ready()
measured_ms = (time.perf_counter() - start) * 1000.0

status = "fast_enough" if measured_ms <= target else "too_slow"
fence = "`" * 3

output = f"""{status}
measured_ms: {measured_ms:.1f}
target_ms: {target:.1f}
benchmark: actual_jax_timing

{fence}python
{candidate}
{fence}"""
```
