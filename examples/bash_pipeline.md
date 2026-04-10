---
name: Bash Pipeline
description: Test end-to-end execution with bash function nodes
external_inputs:
  - message
---

## greeter

```yaml
position: [0, 0]
inputs:
  message: { from: external }
start:
  - always: { max_runs: 1 }
```

```bash
echo "Hello, $message!"
```

## upper

```yaml
position: [1, 0]
inputs:
  text: { from: greeter }
start:
  - when: greeter
```

```bash
echo "$text" | tr '[:lower:]' '[:upper:]'
```

## final

```yaml
position: [2, 0]
inputs:
  shouty: { from: upper }
start:
  - when: upper
```

```bash
echo ">>> $shouty <<<"
```
