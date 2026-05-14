---
name: Test Flow
description: Simple three-node test
external_inputs:
    - name
defaults:
    prompt_executor: codex_cli
    model: gpt-5.3-codex-spark
    temperature: 0.2
---

## greet

```yaml
position: [0, 0]
inputs:
    name: {from: external}
start:
    - always: {max_runs: 1}
```

```bash
echo "hello, $name"
```

## shout

```yaml
position: [2, 0]
inputs:
    msg: {from: greet}
start:
    - when: greet
```

```bash
sleep 2
echo "$msg" | tr '[:lower:]' '[:upper:]'
```

## count

```yaml
position: [4, 0]
inputs:
    msg: {from: shout}
start:
    - when: shout
```

```bash
echo "length: ${#msg}"
```
