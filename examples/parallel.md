---
name: Parallel Fan-Out
description: One source splits into three parallel workers, then joins
external_inputs:
  - input
defaults:
  prompt_executor: codex_cli
  model: gpt-5.3-codex-spark
  temperature: 0.2
---

## source

```yaml
position: [0, 1]
inputs:
  input: { from: external }
start:
  - always: { max_runs: 1 }
```

```bash
echo "$input"
```

## worker_a

```yaml
position: [1, 0]
inputs:
  data: { from: source }
start:
  - when: source
```

```bash
sleep 0.2
echo "A: $data"
```

## worker_b

```yaml
position: [1, 1]
inputs:
  data: { from: source }
start:
  - when: source
```

```bash
sleep 0.2
echo "B: $data"
```

## worker_c

```yaml
position: [1, 2]
inputs:
  data: { from: source }
start:
  - when: source
```

```bash
sleep 0.2
echo "C: $data"
```

## joiner

```yaml
position: [2, 1]
inputs:
  a: { from: worker_a }
  b: { from: worker_b }
  c: { from: worker_c }
start:
  - when: [worker_a, worker_b, worker_c]
```

```bash
echo "joined:"
echo "  $a"
echo "  $b"
echo "  $c"
```
