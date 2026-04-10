---
name: Conditional Branch
description: Reviewer approves or requests changes, different paths fire
external_inputs:
  - decision
---

## reviewer

```yaml
position: [0, 0]
inputs:
  decision: { from: external }
start:
  - always: { max_runs: 1 }
```

```bash
echo "$decision"
```

## approve_path

```yaml
position: [1, 0]
inputs:
  result: { from: reviewer }
start:
  - when: reviewer
    contains: "approved"
```

```bash
echo "APPROVED: merging"
```

## reject_path

```yaml
position: [1, 1]
inputs:
  result: { from: reviewer }
start:
  - when: reviewer
    contains: "rejected"
```

```bash
echo "REJECTED: sending back"
```
