# Flows Examples And Patterns

This reference gives practical patterns for writing, running, and viewing Flows. Use it when creating a new flow, explaining an existing flow, or debugging behavior.

## Mental Model

A flow is a markdown document that becomes an execution graph.

- The frontmatter names the flow and declares external inputs.
- Each top-level `##` section is a block.
- The first fenced `yaml` block in a section configures that block.
- Markdown prose after the config is a prompt block.
- A non-YAML fenced code block after the config is a programmatic block.
- `inputs` specify what data a block receives.
- `start` specifies when the block is eligible to run.
- Runtime outputs become graph edges and are visible in the browser UI.

Good flows make the boundary between fuzzy judgement and deterministic work explicit. Agents should reason, rewrite, decide, or summarize. Code blocks should test, benchmark, parse, score, and enforce conditions.

## Common User Requests

Use these mappings before choosing a longer pattern:

- User says: "Create a flow that summarizes a note."
  Do: one prompt agent, one external input, `start: - always: {max_runs: 1}`.
- User says: "Write JSON, then check it is valid."
  Do: prompt agent for JSON, then a Python `validate_json` block using `json.loads`.
- User says: "Improve this code until it is faster."
  Do: prompt agent plus `benchmark` code block; loop on benchmark output such as `too_slow`.
- User says: "The optimizer's goal is to reduce runtime."
  Do: add a fenced `goal` block inside `## optimizer`; keep benchmarks separate.
- User says: "Have three agents review the same code, then combine their advice."
  Do: three prompt agents reading `from: external`, then a combiner agent with `when: [a, b, c]`.
- User says: "The next agent should know what the previous agent tried."
  Do: require the previous agent to output `HANDOFF:` or structured JSON, then pass that output as a named input.
- User says: "If the loop fails after retries, ask another agent to summarize."
  Do: use `on_exhaustion: summarize_failure` on the loop rule and define `## summarize_failure`.
- User says: "Let the flow modify files."
  Do: use `prompt_executor: codex_cli_write` only on the file-editing agent.
- User says: "Show me the flow."
  Do: run `./flow chart file.md` for the browser chart or `./flow viz file.md` for Mermaid output.

## Minimal Agent Flow

Use this when the user wants a single agent step with one external input.

~~~markdown
---
name: summarize_note
description: Summarize a note in three bullets.
external_inputs:
  - note
defaults:
  prompt_executor: codex_cli
  model: gpt-5.3-codex-spark
---

## summarize

```yaml
inputs:
  note:
    from: external
start:
  - always: {max_runs: 1}
```

Summarize the note in three concise bullets.

Use the provided `note` input.
~~~

Run it:

```bash
./flow validate examples/summarize_note.md
./flow run examples/summarize_note.md -f --input note='A long note goes here'
```

Use `@file` for longer inputs:

```bash
./flow run examples/summarize_note.md -f --input note=@notes/source.md
```

## Agent Then Programmatic Check

Use this when the agent produces something that must be validated by code.

~~~markdown
---
name: json_writer
description: Ask an agent for JSON and validate it programmatically.
external_inputs:
  - topic
defaults:
  prompt_executor: codex_cli
  model: gpt-5.3-codex-spark
---

## write_json

```yaml
inputs:
  topic:
    from: external
start:
  - always: {max_runs: 1}
```

Return only valid JSON with these keys:

- `topic`
- `summary`
- `risks`

Use the provided `topic` input.

## validate_json

```yaml
inputs:
  candidate:
    from: write_json
start:
  - when: write_json
```

```python
import json

data = json.loads(inputs["candidate"])
required = {"topic", "summary", "risks"}
missing = sorted(required - set(data))

output = {
    "ok": not missing,
    "missing": missing,
    "data": data,
}
```
~~~

The validator is deterministic. It should fail loudly if the agent output is malformed.

## Programmatic Block Then Agent

Use this when a code block prepares structured context before an agent sees it.

~~~markdown
---
name: failing_test_explainer
description: Parse test output and ask an agent to explain the failure.
external_inputs:
  - test_output
defaults:
  prompt_executor: codex_cli
  model: gpt-5.3-codex-spark
---

## extract_failures

```yaml
inputs:
  test_output:
    from: external
start:
  - always: {max_runs: 1}
```

```python
lines = inputs["test_output"].splitlines()
failures = [line for line in lines if "FAILED" in line or "ERROR" in line]
output = {
    "failure_count": len(failures),
    "failures": failures[:20],
}
```

## explain

```yaml
inputs:
  failures:
    from: extract_failures
start:
  - when: extract_failures
```

Explain the likely root cause and the smallest next debugging step.

Parsed failures:

Use the provided `failures` input.
~~~

## Multi-Agent Review

Use this pattern when several agents should inspect the same artifact with different responsibilities, then a later block combines their results.

~~~markdown
---
name: code_review_panel
description: Review code for speed, memory, and clarity.
external_inputs:
  - code
defaults:
  prompt_executor: codex_cli
  model: gpt-5.3-codex-spark
---

## speed_review

```yaml
inputs:
  code:
    from: external
start:
  - always: {max_runs: 1}
```

Review this code only for speed. Return specific changes and expected impact.

Use the provided `code` input.

## memory_review

```yaml
inputs:
  code:
    from: external
start:
  - always: {max_runs: 1}
```

Review this code only for memory use. Return specific changes and expected impact.

Use the provided `code` input.

## clarity_review

```yaml
inputs:
  code:
    from: external
start:
  - always: {max_runs: 1}
```

Review this code only for conciseness and wasted complexity. Do not optimize at the expense of correctness.

Use the provided `code` input.

## combine_reviews

```yaml
inputs:
  speed:
    from: speed_review
  memory:
    from: memory_review
  clarity:
    from: clarity_review
start:
  - when: [speed_review, memory_review, clarity_review]
```

Combine the three reviews into one prioritized patch plan. Deduplicate repeated advice and flag conflicts.
~~~

This gives each agent a narrow role. The final agent resolves conflicts rather than letting every agent solve the whole problem.

## Feedback Loop With A Code Gate

Use this when agent output should be repeatedly improved until a deterministic condition passes.

~~~markdown
---
name: make_fast_enough
description: Improve code until the benchmark passes or the loop limit is reached.
external_inputs:
  - code
  - target_seconds
defaults:
  prompt_executor: codex_cli
  model: gpt-5.3-codex-spark
---

## improve

```yaml
inputs:
  code:
    from: external
    fallback: benchmark
  benchmark_result:
    from: benchmark
    fallback: external
start:
  - always: {max_runs: 1}
  - when: benchmark
    contains: too_slow
    max_runs: 4
```

Rewrite the code to make it faster while preserving behavior.

If benchmark feedback is available, use it. Return only the complete revised code.

Use the provided `code` input. If benchmark feedback is available in
`benchmark_result`, use it to guide the next revision.

## benchmark

```yaml
inputs:
  code:
    from: improve
  target_seconds:
    from: external
start:
  - when: improve
```

```python
import json
import time

namespace = {}
started = time.perf_counter()
exec(inputs["code"], namespace)
elapsed = time.perf_counter() - started
target = float(inputs["target_seconds"])

result = {
    "passed": elapsed < target,
    "elapsed": elapsed,
    "target_seconds": target,
    "code": inputs["code"],
}
status = "fast_enough" if result["passed"] else "too_slow"
output = status + "\n" + json.dumps(result)
```
~~~

Important loop details:

- The improvement agent starts once from the external code.
- After benchmark runs, the same agent can run again when the output contains `too_slow`.
- `max_runs` prevents infinite loops.
- The code gate decides whether the loop should continue.
- The UI should show the loop condition clearly because it is the main control edge.

## Explicit State Handoff

Use this pattern when a later agent needs more than a final answer. Flows do
not transfer chat history, hidden reasoning, or tool transcripts between
blocks. Only declared inputs are passed, and each upstream block contributes
its latest raw output text.

Ask the upstream agent to emit a deliberate handoff artifact:

~~~markdown
## investigate

```yaml
inputs:
  bug_report:
    from: external
start:
  - always: {max_runs: 1}
```

Investigate the bug report. Return:

RESULT:
- Most likely cause
- Evidence

HANDOFF:
- What you tried
- Important assumptions
- Failed approaches
- Suggested next action
~~~

Then pass that output explicitly:

~~~markdown
## fix_plan

```yaml
inputs:
  investigation:
    from: investigate
start:
  - when: investigate
```

Use the `investigation` input. Preserve the HANDOFF assumptions unless you find
contradicting evidence. Produce a minimal patch plan.
~~~

Prefer this over asking agents to rely on implicit prior context.

## Loop Exhaustion Route

Use this when a loop reaches `max_runs` and should call another agent instead
of stopping the whole flow or silently continuing. `stop` and `continue` are
reserved policies; any other valid agent name is a route target.

~~~markdown
---
name: optimization_with_escalation
description: Optimize until benchmark passes, then escalate if the loop exhausts.
external_inputs:
  - code
  - target_ms
defaults:
  prompt_executor: codex_cli
  model: gpt-5.3-codex-spark
---

## improve

```yaml
inputs:
  code:
    from: benchmark
    fallback: external
  target_ms:
    from: external
start:
  - always: {max_runs: 1}
  - when: benchmark
    contains: too_slow
    max_runs: 3
    on_exhaustion: escalate
```

Improve the code. If benchmark feedback is present in `code`, use the latest
fenced code block and the feedback as the starting point.

Return only one fenced Python code block.

## benchmark

```yaml
inputs:
  candidate:
    from: improve
  target_ms:
    from: external
start:
  - when: improve
```

```python
import json

# Replace this with a real benchmark in production flows.
passed = "fast_path" in inputs["candidate"]
status = "fast_enough" if passed else "too_slow"
output = status + "\n" + json.dumps({
    "passed": passed,
    "target_ms": inputs["target_ms"],
    "candidate": inputs["candidate"],
})
```

## escalate

```yaml
inputs:
  latest_feedback:
    from: benchmark
  original_code:
    from: external
```

The optimizer exhausted its retry budget.

Summarize:
- Why the loop did not finish
- The latest benchmark feedback
- The safest next manual or agentic step
~~~

Route-only handlers such as `escalate` may omit ordinary `start` conditions if
they are only reached through `on_exhaustion`. The runtime fires the handler
once, after confirming its inputs are available. Mermaid charts show the
control edge as `on exhaustion`.

Use `on_exhaustion: continue` only when exhausting the loop should not be
considered an error and no handler is needed.

## Goal-Style Agent Blocks

Use a fenced `goal` block immediately after an agent's YAML config when the
user asks for a goal attached to that specific agent. Common requests include
"set up a goal", "the goal of this agent should be...", "give `optimizer` a
goal", or "this agent should keep working toward...".

A goal block is metadata attached to one owning agent. In the browser chart it
appears as a stacked goal card above that agent with a small `<->` association
line. It is not a separate executable block, not a benchmark, and not a graph
loop condition.

Use goal cards for per-agent intent:

- "The goal of `optimizer` is to make the code faster" means add a `goal` block inside `## optimizer`.
- "The reviewer agent's goal is to find safety issues" means add a `goal` block inside `## reviewer`.
- "Set up a goal for the writer" means attach the goal to the writer agent card, not to the whole flow.

~~~markdown
## optimizer

```yaml
inputs:
  code:
    from: external
  target_ms:
    from: external
start:
  - always: {max_runs: 1}
prompt_executor: codex_cli
```

```goal
objective: Optimize the JAX code until it runs under target_ms.
validation:
  - Return a complete fenced Python block.
  - Preserve public function names and behavior.
  - Explain any remaining benchmark risk in the handoff note.
max_turns: 5
on_exhaustion: escalate
```

Use the latest fenced Python block in `code` as the starting point. Preserve
public function names and behavior. Return only the final fenced Python code
block followed by a short handoff note.
~~~

The runtime injects the goal objective and validation criteria into the prompt
sent to the agent. The browser UI renders the goal as a stacked metadata card
above the agent with a small `<->` association line, so it does not look like a
normal executable block.

Goal `validation` text is an agent-facing contract. It tells the agent what
"done" should mean for that agent. It does not replace deterministic tests,
benchmarks, or parser checks. If the flow must prove something with code, add a
normal programmatic block.

### Programmatic Loop Gate Is Separate

When the user asks for a loop of several agents followed by a concrete check,
use ordinary flow blocks and a code gate. For example:

> Define a loop of several agents and at the end check programmatically that the
> code is faster.

This is not a single goal card. Model it as agent blocks plus a deterministic
`benchmark` block. The benchmark output drives the loop:

~~~markdown
## speed_agent

```yaml
inputs:
  code:
    from: external
    fallback: benchmark
  benchmark_result:
    from: benchmark
    fallback: external
start:
  - always: {max_runs: 1}
  - when: benchmark
    contains: too_slow
    max_runs: 4
```

Rewrite the code to make it faster while preserving behavior. Use benchmark
feedback when it is available.

## cleanup_agent

```yaml
inputs:
  code:
    from: speed_agent
start:
  - when: speed_agent
```

Clean up the implementation without making it slower.

## benchmark

```yaml
inputs:
  code:
    from: cleanup_agent
  target_seconds:
    from: external
start:
  - when: cleanup_agent
```

```python
import time

namespace = {}
started = time.perf_counter()
exec(inputs["code"], namespace)
elapsed = time.perf_counter() - started
target = float(inputs["target_seconds"])

status = "fast_enough" if elapsed < target else "too_slow"
output = f"{status}\nelapsed={elapsed}\ntarget={target}\n" + inputs["code"]
```
~~~

The `benchmark` block is a normal executable card in the graph. Its output text
contains `too_slow` or `fast_enough`; the `speed_agent` start rule uses that
text to decide whether to loop. This is the right pattern for measurable
runtime, correctness, JSON validity, test pass/fail, coverage thresholds, and
other checks that should be enforced by code.

### Combining Both Patterns

Goal cards and code gates can coexist. If the user says:

> Give the speed agent a goal to minimize runtime, then loop through cleanup and
> benchmark until the code is fast enough.

Do both:

- Add a `goal` block inside `## speed_agent` because the goal belongs to that one agent card.
- Add a separate `## benchmark` code block because "fast enough" must be measured programmatically.
- Use `start` conditions on `speed_agent` or another agent to loop when `benchmark` outputs `too_slow`.

For future multi-turn headless Codex-backed goals, do not send `/goal` through
`codex exec`. Instead, implement a goal executor that:

1. Starts a persistent `codex exec --json` thread without `--ephemeral`.
2. Captures the exact `thread_id` from the JSONL stream.
3. Asks Codex to return structured status such as `complete`, `continue`, or `blocked`.
4. Calls `codex exec resume <thread_id> "Continue toward the same goal..."` when status is `continue`.
5. Stops on `complete`, `blocked`, `max_turns`, timeout, or an exhaustion policy.

Never use `codex exec resume --last` inside Flows; parallel blocks make it
unsafe. Always resume the captured thread ID for the block.

## JAX Optimization Example

This is the canonical real-life example. It starts with real but poorly optimized JAX code and sends it through agents with distinct responsibilities:

1. Minimize runtime.
2. Minimize memory.
3. Remove waste and improve conciseness.
4. Benchmark the result.
5. Loop until the code runs under a target time or the run limit is reached.

Skeleton:

~~~markdown
---
name: jax_optimization_loop
description: Optimize simple JAX code through specialist agents and a benchmark loop.
external_inputs:
  - code
  - target_ms
defaults:
  prompt_executor: codex_cli
  model: gpt-5.3-codex-spark
---

## speed_agent

```yaml
inputs:
  code:
    from: external
    fallback: benchmark
  benchmark:
    from: benchmark
    fallback: external
start:
  - always: {max_runs: 1}
  - when: benchmark
    contains: too_slow
    max_runs: 4
prompt_executor: codex_cli
model: gpt-5.3-codex-spark
```

You are optimizing JAX code for minimum runtime.

Return only complete Python code. Preserve the original result semantics.

Use the provided `code` input. If benchmark feedback is available in
`benchmark`, use it to guide the next revision.

## memory_agent

```yaml
inputs:
  code:
    from: speed_agent
start:
  - when: speed_agent
prompt_executor: codex_cli
model: gpt-5.3-codex-spark
```

You are optimizing JAX code for lower peak memory use. Preserve speed improvements where possible.

Return only complete Python code.

Use the provided `code` input.

## concision_agent

```yaml
inputs:
  code:
    from: memory_agent
start:
  - when: memory_agent
prompt_executor: codex_cli
model: gpt-5.3-codex-spark
```

Remove waste, duplication, and unnecessary complexity. Preserve runtime behavior.

Return only complete Python code.

Use the provided `code` input.

## benchmark

```yaml
inputs:
  code:
    from: concision_agent
  target_ms:
    from: external
start:
  - when: concision_agent
```

```python
import json
import subprocess
import sys
import tempfile
import time
from pathlib import Path

code = inputs["code"]
target = float(inputs["target_ms"])

with tempfile.TemporaryDirectory() as tmp:
    path = Path(tmp) / "candidate.py"
    path.write_text(code)
    started = time.perf_counter()
    proc = subprocess.run(
        [sys.executable, str(path)],
        text=True,
        capture_output=True,
        timeout=max(10.0, target * 10.0),
    )
    elapsed = time.perf_counter() - started

result = {
    "passed": proc.returncode == 0 and elapsed < target,
    "elapsed": elapsed,
    "target_ms": target,
    "stdout": proc.stdout[-4000:],
    "stderr": proc.stderr[-4000:],
    "code": code,
}
status = "fast_enough" if result["passed"] else "too_slow"
output = status + "\n" + json.dumps(result)
```
~~~

Run the real example:

```bash
./flow validate examples/jax_optimization_loop.md
./flow run examples/jax_optimization_loop.md -f --input code=@examples/inputs/slow_jax.py --input target_ms=5
```

Use `--dry-run` when checking structure without running agents:

```bash
./flow run examples/jax_optimization_loop.md --dry-run --input code=@examples/inputs/slow_jax.py --input target_ms=5
```

## Choosing Prompt Executors

Prompt executor choice belongs in the flow defaults or in each prompt block config.

Common pattern for quick examples:

```yaml
defaults:
  prompt_executor: codex_cli
  model: gpt-5.3-codex-spark
```

Use write-capable Codex only for nodes whose prompt explicitly asks the agent
to edit files in the repository:

```yaml
prompt_executor: codex_cli_write
model: gpt-5.3-codex-spark
```

Per-block override:

```yaml
prompt_executor: codex_cli
model: gpt-5.3-codex-spark
temperature: 0
```

Use API executors only when explicitly desired:

```yaml
prompt_executor: anthropic_api
model: claude-3-5-sonnet-latest
```

When using CLI-backed execution, the user may already be authenticated in the local CLI. API executors need API keys in the environment.

## Viewing A Flow

The browser UI is part of normal execution.

```bash
./flow run examples/jax_optimization_loop.md --input code=@examples/inputs/slow_jax.py --input target_ms=5
```

This prints a `View flow:` link and runs in the background.

```bash
./flow run examples/jax_optimization_loop.md -f --input code=@examples/inputs/slow_jax.py --input target_ms=5
```

This prints the same link and tails live execution.

Static graph commands:

```bash
./flow chart examples/jax_optimization_loop.md
./flow viz examples/jax_optimization_loop.md
```

Use the browser graph to inspect:

- Block type: agent, code, or input/output.
- Input blocks pointing into the block that consumes them.
- Output prompts or generated outputs pointing to downstream consumers.
- Loop labels and conditions.
- Block details by clicking blocks, inputs, and outputs.

## Debugging Checklist

When a flow fails to parse:

- Check that frontmatter exists and contains `name`.
- Check every `##` block has a first fenced `yaml` config block.
- Check indentation in YAML.
- Check `inputs` is a map, not a list.
- Check `start.when` names an existing block or a list of existing blocks.

When a flow does not run in the expected order:

- Inspect `start` on the block that did not run.
- Confirm the upstream block produced output.
- Confirm the loop condition matches the actual structured output.
- Add a deterministic diagnostic code block if the condition depends on text.

When an agent sees the wrong input:

- Prefer named inputs over relying on prior context.
- Avoid ambiguous names like `result`; use `benchmark_result`, `review`, or `candidate_code`.
- Use `fallback` only when the first iteration genuinely has no upstream output.
- Keep large code or logs in files and pass with `--input name=@path`.

When real execution is required:

- Use real source code, not placeholders like `some_code`.
- Validate dependencies are installed before running the flow.
- Keep programmatic blocks deterministic.
- Put timeout and return-code checks around subprocess benchmarks.
- Include enough stdout and stderr in `output` for downstream agents to diagnose failures.

## Style Guidance

Good flow files are readable as documents before they are executed.

- Give blocks action-oriented names: `speed_agent`, `benchmark`, `summarize_failures`.
- Keep each agent prompt focused on one responsibility; use a fenced `goal` block only when the user wants a visible goal card attached to that agent.
- Make all cross-block data movement explicit.
- Put deterministic checks in code blocks instead of asking agents to self-report success.
- Use small loop limits unless the user explicitly wants long autonomous runs.
- Prefer concise Markdown prompts with clear output requirements.
- Avoid hidden fallbacks or implicit behavior when the graph can express it directly.
