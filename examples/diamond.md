---
name: Diamond DAG
description: Deep diamond-shaped dependency graph
external_inputs:
  - seed
defaults:
  prompt_executor: codex_cli
  model: gpt-5.3-codex-spark
  temperature: 0.2
---

## a

```yaml
position: [0, 2]
inputs:
  seed: { from: external }
start:
  - always: { max_runs: 1 }
```

```bash
echo "$seed"
```

## b1

```yaml
position: [1, 0]
inputs:
  v: { from: a }
start:
  - when: a
```

```bash
echo "$((v * 2))"
```

## b2

```yaml
position: [1, 2]
inputs:
  v: { from: a }
start:
  - when: a
```

```bash
echo "$((v + 10))"
```

## b3

```yaml
position: [1, 4]
inputs:
  v: { from: a }
start:
  - when: a
```

```bash
echo "$((v * v))"
```

## c1

```yaml
position: [2, 1]
inputs:
  b1: { from: b1 }
  b2: { from: b2 }
start:
  - when: [b1, b2]
```

```bash
echo "$((b1 + b2))"
```

## c2

```yaml
position: [2, 3]
inputs:
  b2: { from: b2 }
  b3: { from: b3 }
start:
  - when: [b2, b3]
```

```bash
echo "$((b2 + b3))"
```

## final

```yaml
position: [3, 2]
inputs:
  c1: { from: c1 }
  c2: { from: c2 }
start:
  - when: [c1, c2]
```

```bash
echo "final: c1=$c1 c2=$c2 total=$((c1 + c2))"
```
