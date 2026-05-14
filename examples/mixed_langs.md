---
name: Mixed Languages
description: Bash, Python working together on data
external_inputs:
  - numbers
defaults:
  prompt_executor: codex_cli
  model: gpt-5.3-codex-spark
  temperature: 0.2
---

## parser

```yaml
position: [0, 0]
inputs:
  numbers: { from: external }
start:
  - always: { max_runs: 1 }
```

```python
nums = [int(x) for x in numbers.split(",")]
stats = {
    "count": len(nums),
    "sum": sum(nums),
    "mean": sum(nums) / len(nums),
    "max": max(nums),
}
import json
output = json.dumps(stats)
```

## formatter

```yaml
position: [1, 0]
inputs:
  stats: { from: parser }
start:
  - when: parser
```

```bash
echo "Stats: $stats"
```

## filter

```yaml
position: [1, 1]
inputs:
  numbers: { from: external }
start:
  - always: { max_runs: 1 }
```

```python
nums = [int(x) for x in numbers.split(",")]
output = ",".join(str(n) for n in nums if n % 2 == 0)
```

## reporter

```yaml
position: [2, 0]
inputs:
  formatted: { from: formatter }
  evens: { from: filter }
start:
  - when: [formatter, filter]
```

```bash
echo "=== REPORT ==="
echo "$formatted"
echo "Even numbers: $evens"
```
