---
name: Simple Goal Demo
description: A minimal flow showing an attached goal block rendered separately from the agent prompt
external_inputs:
    - topic
defaults:
    prompt_executor: codex_cli
    model: gpt-5.3-codex-spark
    temperature: 0.2
---

## goal_writer

```yaml
position: [8, 1]
inputs:
    topic: {from: external}
start:
    - always: {max_runs: 1}
```

```goal
objective: Write exactly three concise action bullets for the provided topic.
validation:
    - Return exactly three bullet lines.
    - Each line starts with "- ".
    - Each line contains at most 12 words.
    - Do not include headings, preamble, or follow-up text.
max_turns: 1
```

Use the provided `topic` input.
