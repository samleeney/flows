---
name: JAX Flash Demo
description: A compact JAX optimization flow with a correctness loop inside a speed loop, plus a separate polish loop.
external_inputs:
    - code
    - target_ms
defaults:
    prompt_executor: codex_cli
    model: gpt-5.3-codex-spark
    temperature: 0.2
---

## speed

```yaml
position: [8, 4]
inputs:
    code: {from: check, fallback: external}
    target_ms: {from: external}
start:
    - always: {max_runs: 1}
    - when: check
      contains: correctness_fail
      max_runs: 2
    - when: bench
      contains: too_slow
      max_runs: 4
```

Optimize this JAX code for runtime.

Use the latest fenced Python block in `code` as the starting point. If `code`
contains `correctness_fail`, fix behavior first. Otherwise, make it faster.

Rules:
- Preserve `loss(x, w)` and its numerical result.
- Prefer `jax.jit`, `jax.vmap`, and fused `jax.numpy` operations.
- Remove row-wise Python loops over JAX arrays.

Return only one fenced `python` block with the complete code.

## check

```yaml
position: [16, 5]
inputs:
    candidate: {from: speed}
    code: {from: external}
start:
    - when: speed
```

```python
import contextlib
import io
import re
import traceback

import jax.numpy as jnp


def latest_python_block(text):
    blocks = re.findall(r"```(?:python)?\n(.*?)```", text, flags=re.S)
    return blocks[-1].strip() if blocks else text.strip()


candidate_code = latest_python_block(candidate)
original_code = latest_python_block(code)
status = "correctness_ok"
notes = []

try:
    original_ns = {}
    candidate_ns = {}
    exec(original_code, original_ns)
    exec(candidate_code, candidate_ns)

    x = jnp.reshape(jnp.linspace(-1.0, 1.0, 64 * 32), (64, 32))
    w = jnp.linspace(0.1, 1.0, 32)

    with contextlib.redirect_stdout(io.StringIO()):
        expected = original_ns["loss"](x, w).block_until_ready()
        actual = candidate_ns["loss"](x, w).block_until_ready()

    if not bool(jnp.allclose(expected, actual, rtol=1e-4, atol=1e-4)):
        status = "correctness_fail"
        notes.append(f"expected={float(expected):.6f}")
        notes.append(f"actual={float(actual):.6f}")
except Exception:
    status = "correctness_fail"
    notes.append(traceback.format_exc(limit=4))

fence = "`" * 3
output = f"""{status}
notes:
{chr(10).join("- " + note for note in notes) if notes else "- candidate matches original loss"}

{fence}python
{candidate_code}
{fence}"""
```

## bench

```yaml
position: [33, 6]
inputs:
    candidate: {from: check}
    target_ms: {from: external}
start:
    - when: check
      contains: correctness_ok
```

```python
import contextlib
import io
import re
import time

import jax.numpy as jnp


def latest_python_block(text):
    blocks = re.findall(r"```(?:python)?\n(.*?)```", text, flags=re.S)
    return blocks[-1].strip() if blocks else text.strip()


candidate_code = latest_python_block(candidate)
target = float(target_ms)
namespace = {}
exec(candidate_code, namespace)
loss_fn = namespace["loss"]

x = jnp.ones((256, 128), dtype=jnp.float32)
w = jnp.linspace(0.1, 1.0, 128, dtype=jnp.float32)

with contextlib.redirect_stdout(io.StringIO()):
    for _ in range(2):
        loss_fn(x, w).block_until_ready()

    start = time.perf_counter()
    loss_fn(x, w).block_until_ready()
    measured_ms = (time.perf_counter() - start) * 1000.0

status = "fast_enough" if measured_ms <= target else "too_slow"
fence = "`" * 3
output = f"""{status}
measured_ms: {measured_ms:.2f}
target_ms: {target:.2f}

{fence}python
{candidate_code}
{fence}"""
```

## polish

```yaml
position: [49, 4]
inputs:
    code: {from: final, fallback: bench}
    target_ms: {from: external}
start:
    - when: bench
      contains: fast_enough
    - when: final
      contains: polish_needed
      max_runs: 2
```

Clean up the optimized JAX code without changing behavior or slowing it down.

Use the latest fenced Python block in `code` as the starting point. Remove debug
prints, unused helpers, dead variables, and noisy comments. Keep `loss(x, w)`
intact.

Return only one fenced `python` block with the complete code.

## final

```yaml
position: [57, 4]
inputs:
    candidate: {from: polish}
    code: {from: external}
    target_ms: {from: external}
start:
    - when: polish
```

```python
import contextlib
import io
import re
import time
import traceback

import jax.numpy as jnp


def latest_python_block(text):
    blocks = re.findall(r"```(?:python)?\n(.*?)```", text, flags=re.S)
    return blocks[-1].strip() if blocks else text.strip()


candidate_code = latest_python_block(candidate)
original_code = latest_python_block(code)
target = float(target_ms)
issues = []
measured_ms = None

try:
    original_ns = {}
    candidate_ns = {}
    exec(original_code, original_ns)
    exec(candidate_code, candidate_ns)

    x_small = jnp.reshape(jnp.linspace(-1.0, 1.0, 64 * 32), (64, 32))
    w_small = jnp.linspace(0.1, 1.0, 32)

    with contextlib.redirect_stdout(io.StringIO()):
        expected = original_ns["loss"](x_small, w_small).block_until_ready()
        actual = candidate_ns["loss"](x_small, w_small).block_until_ready()

    if not bool(jnp.allclose(expected, actual, rtol=1e-4, atol=1e-4)):
        issues.append("correctness changed")

    x = jnp.ones((256, 128), dtype=jnp.float32)
    w = jnp.linspace(0.1, 1.0, 128, dtype=jnp.float32)

    with contextlib.redirect_stdout(io.StringIO()):
        for _ in range(2):
            candidate_ns["loss"](x, w).block_until_ready()
        start = time.perf_counter()
        candidate_ns["loss"](x, w).block_until_ready()
        measured_ms = (time.perf_counter() - start) * 1000.0

    if measured_ms > target:
        issues.append(f"runtime regressed to {measured_ms:.2f} ms")
    if "print(" in candidate_code:
        issues.append("debug print remains")
    if re.search(r"for\s+\w+\s+in\s+range\s*\(\s*x\.shape\[0\]\s*\)", candidate_code):
        issues.append("row-wise Python loop remains")
except Exception:
    issues.append(traceback.format_exc(limit=4))

status = "demo_ready" if not issues else "polish_needed"
fence = "`" * 3
output = f"""{status}
measured_ms: {measured_ms if measured_ms is not None else "unavailable"}
issues:
{chr(10).join("- " + issue for issue in issues) if issues else "- none"}

{fence}python
{candidate_code}
{fence}"""
```
