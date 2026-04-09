---
name: Function Node Flow
description: Flow with a function node
external_inputs:
  - raw_data
---

## processor

```yaml
position: [0, 0]
inputs:
  data: { from: external }
start:
  - always: { max_runs: 1 }
```

```python
import json
data = json.loads(raw_data)
output = [d for d in data if d["score"] > 0.5]
```

## reporter

```yaml
position: [1, 0]
inputs:
  results: { from: processor }
start:
  - when: processor
```

Summarise the processed results.
