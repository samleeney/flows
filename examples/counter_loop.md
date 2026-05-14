---
name: Counter Loop
description: Increment a counter until it says done
external_inputs:
  - value
defaults:
  prompt_executor: codex_cli
  model: gpt-5.3-codex-spark
  temperature: 0.2
---

## counter

```yaml
position: [0, 0]
inputs:
  value: { from: incrementer, fallback: external }
start:
  - always: { max_runs: 1 }
  - when: incrementer
    max_runs: 5
```

```bash
n=$value
if [ "$n" -ge 3 ]; then
  echo "done: $n"
else
  echo "continue: $n"
fi
```

## incrementer

```yaml
position: [1, 0]
inputs:
  status: { from: counter }
start:
  - when: counter
    contains: "continue"
```

```bash
# Extract number from "continue: N" and add 1
n=$(echo "$status" | awk '{print $2}')
echo $((n + 1))
```

## reporter

```yaml
position: [2, 0]
inputs:
  final: { from: counter }
start:
  - when: counter
    contains: "done"
```

```bash
echo "Final state: $final"
```
