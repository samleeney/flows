---
name: Function Node Flow
description: Flow with a function node
external_inputs:
  - data
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
parsed = json.loads(data)
output = [d for d in parsed if d["score"] > 0.5]
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
