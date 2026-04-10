---
name: Failure Test
description: Agent with non-zero exit
external_inputs:
  - input
---

## good_one

```yaml
position: [0, 0]
inputs:
  input: { from: external }
start:
  - always: { max_runs: 1 }
```

```bash
echo "ok: $input"
```

## bad_one

```yaml
position: [1, 0]
inputs:
  data: { from: good_one }
start:
  - when: good_one
```

```bash
echo "about to fail" >&2
exit 1
```

## never_reached

```yaml
position: [2, 0]
inputs:
  data: { from: bad_one }
start:
  - when: bad_one
```

```bash
echo "should not run"
```
