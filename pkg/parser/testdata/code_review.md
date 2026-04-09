---
name: Code Review Flow
description: Automated review-fix loop with merge
external_inputs:
  - code
  - guidelines
defaults:
  model: claude-sonnet-4-20250514
  temperature: 0.3
---

## reviewer

```yaml
position: [0, 0]
inputs:
  code: { from: fixer, fallback: external }
  guidelines: { from: external }
start:
  - always: { max_runs: 1 }
  - when: fixer
    max_runs: 5
```

Review the provided code against the guidelines.

Focus on:
- Correctness
- Security vulnerabilities
- Style violations

Your first line of output MUST be either `needs_changes` or `approved`.

## fixer

```yaml
position: [1, 1]
inputs:
  code: { from: external }
  feedback: { from: reviewer }
start:
  - when: reviewer
    contains: "needs_changes"
```

You are given code and review feedback. Fix every issue raised.

Return the complete corrected code. Do not explain your changes.

## merger

```yaml
position: [2, 0]
inputs:
  code: { from: fixer, fallback: external }
start:
  - when: reviewer
    contains: "approved"
```

Merge this code. Summarise what was changed during review.
