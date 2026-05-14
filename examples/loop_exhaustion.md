---
name: Loop Exhaustion
description: Loop that hits max_runs without satisfying exit condition
external_inputs:
  - value
defaults:
  prompt_executor: codex_cli
  model: gpt-5.3-codex-spark
  temperature: 0.2
---

## checker

```yaml
position: [0, 0]
inputs:
  value: { from: changer, fallback: external }
start:
  - always: { max_runs: 1 }
  - when: changer
    max_runs: 3
```

```bash
# Never says "done" — forces max_runs to hit
echo "still_going: $value"
```

## changer

```yaml
position: [1, 0]
inputs:
  status: { from: checker }
start:
  - when: checker
    contains: "still_going"
```

```bash
echo "$(date +%N)"
```

## final

```yaml
position: [2, 0]
inputs:
  result: { from: checker }
start:
  - when: checker
    contains: "done"
```

```bash
echo "Done: $result"
```
