# JAX One-Loop Demo

This walkthrough uses the two-block JAX optimization flow. Run each command
from the root of the Flows repository.

## 1. Install the prerequisites

The demo requires Go, an authenticated Codex CLI, and Python with JAX:

```bash
python3 -m venv .venv
.venv/bin/python -m pip install "jax[cpu]"
```

## 2. Build the CLI

```bash
go build -o flow ./cmd/flow
```

## 3. Validate the flow

```bash
./flow validate examples/jax_short_goal_loop.md
```

Expected output:

```text
Flow "Short JAX Speed Goal Loop" is valid (2 agents)
```

## 4. Open the visual editor

```bash
./flow chart examples/jax_short_goal_loop.md > /tmp/flow-chart.log 2>&1 &
```

If the browser does not open automatically, add `--no-open` and open
`http://127.0.0.1:8420` yourself:

```bash
./flow chart examples/jax_short_goal_loop.md --no-open > /tmp/flow-chart.log 2>&1 &
```

Use `--port 8421` if port 8420 is already occupied.

## 5. Dry-run the execution plan

```bash
./flow run examples/jax_short_goal_loop.md --dry-run \
  --input code=@examples/inputs/slow_jax.py \
  --input target_ms=5
```

## 6. Run the demo live

```bash
FLOW_PYTHON_COMMAND=.venv/bin/python \
  ./flow run examples/jax_short_goal_loop.md -f \
  --input code=@examples/inputs/slow_jax.py \
  --input target_ms=5
```

The chart process writes its browser link to `/tmp/flow-chart.log`. The
foreground run prints a browser link and streams progress until the flow ends.
