# PRD: Machine-Friendly Flows CLI

## 1. Introduction / Overview

Flows currently provides human-oriented commands for validation, execution,
visualization, and the browser chart. Those commands are usable by coding
agents through a shell, but their output is prose, foreground and background
execution have different side effects, and `flow run` starts an editor even
when the caller only needs a result.

This feature adds a stable machine-facing contract without replacing or
breaking the existing human CLI. Agents and scripts will be able to provide a
flow through a file or standard input, inspect its parsed structure, validate
it, dry-run it, execute it without starting the editor, and consume either one
JSON result or a stream of JSON Lines events.

The existing repository-scoped Flows skill will be updated to prefer these
interfaces for agent-driven work. A second overlapping skill will not be
created.

## 2. Goals

1. Provide JSON output that callers can parse without scraping prose.
2. Provide JSON Lines progress for long-running foreground executions.
3. Keep stdout machine-clean in structured modes and reserve stderr for
   diagnostics that are not part of the result contract.
4. Allow flow source and external inputs to be supplied without fragile shell
   quoting.
5. Avoid browser/editor startup and detached child processes in machine mode.
6. Preserve the existing text-mode CLI and its current defaults.
7. Teach Codex and other skill-aware agents when and how to use the new
   interface.

## 3. User Stories

### US-001: Shared structured-output contract

**Description:** As an automation author, I want every structured CLI response
to use a versioned envelope so that my parser can detect success, failure, and
future schema changes reliably.

**Acceptance Criteria:**

- [ ] A shared package defines versioned JSON result, error, diagnostic, flow
      summary, block summary, and output types; command packages do not expose
      internal Go structs as the public wire format.
- [ ] Every JSON document contains `schema_version`, `command`, and `ok`.
- [ ] Structured errors contain a stable `code`, a human-readable `message`,
      and optional `details` or `diagnostics`.
- [ ] Successful commands exit 0; invalid arguments or invalid flow/input data
      exit 2; execution failures exit 3; unexpected internal failures exit 1.
- [ ] In JSON and JSONL modes, stdout contains only valid JSON documents or
      JSON Lines records.
- [ ] Existing text output and exit behavior remain compatible by default.
- [ ] Unit tests, `go test ./...`, and `go vet ./...` pass.

### US-002: Flow source from standard input

**Description:** As an agent generating a draft flow, I want to validate or
inspect source from stdin so that I do not need to create a temporary file.

**Acceptance Criteria:**

- [ ] `flow validate -`, `flow inspect -`, `flow viz -`, and `flow run -`
      read the flow Markdown from stdin.
- [ ] File-based behavior remains unchanged for ordinary paths.
- [ ] The parser is invoked through a shared source loader rather than
      command-specific stdin implementations.
- [ ] Commands reject configurations that assign stdin to more than one
      consumer, such as flow source `-` together with `--inputs-json -`.
- [ ] Parse and validation errors identify the source as `<stdin>` rather than
      an empty or temporary path.
- [ ] E2E tests cover valid and invalid stdin flows.
- [ ] `go test ./...` passes.

### US-003: JSON validation

**Description:** As an agent editing a flow, I want validation diagnostics as
JSON so that I can correct a draft deterministically.

**Acceptance Criteria:**

- [ ] `flow validate <file|-> --json` emits exactly one JSON result document.
- [ ] A valid result includes a flow summary with name, description, external
      inputs, block count, and source identity.
- [ ] An invalid result has `ok: false`, a non-zero exit code, and diagnostics
      identifying the `parse` or `validation` phase.
- [ ] Diagnostics are arrays even when only one issue is available.
- [ ] Human text is not mixed into stdout in JSON mode.
- [ ] Existing `flow validate <file>` output remains unchanged.
- [ ] E2E tests parse stdout with `encoding/json` instead of substring
      matching.
- [ ] `go test ./...` passes.

### US-004: Parsed flow inspection

**Description:** As an agent reasoning about an existing flow, I want a stable
JSON representation of its graph so that I do not have to reimplement the
Markdown parser.

**Acceptance Criteria:**

- [ ] A new `flow inspect <file|-> --json` command parses and validates the
      flow without executing it.
- [ ] The result includes flow metadata, defaults, external inputs, and blocks
      in source order.
- [ ] Each block includes name, node kind, language, position, declared inputs,
      start conditions, goal metadata, and executor/model overrides where
      present.
- [ ] Input and control dependencies are represented explicitly enough for a
      caller to reconstruct the graph.
- [ ] Prompt or code content is included in a documented field rather than
      silently omitted.
- [ ] The JSON DTO uses stable string enums such as `prompt` and `function`,
      not internal numeric enum values.
- [ ] Invalid flows use the shared structured error contract.
- [ ] Unit and E2E tests cover prompt blocks, function blocks, loops, fallbacks,
      and goal cards.
- [ ] `go test ./...` passes.

### US-005: Machine-readable dry runs

**Description:** As an automation author, I want a JSON dry-run result so that
I can verify execution inputs and planned blocks without launching agents.

**Acceptance Criteria:**

- [ ] `flow run <file|-> --dry-run --json` emits one JSON document and performs
      no prompt, Bash, or Python execution.
- [ ] The result contains the validated flow summary, required external input
      names, supplied input names, block count, and execution mode.
- [ ] Missing required inputs produce a structured failure rather than a
      successful dry run.
- [ ] Dry-run JSON mode never starts or discovers an editor and never creates
      a detached child process.
- [ ] Existing text dry-run output remains unchanged unless the missing-input
      validation is intentionally made consistent and documented.
- [ ] E2E tests prove that executors and editor startup are not invoked.
- [ ] `go test ./...` passes.

### US-006: Foreground JSON execution

**Description:** As an agent invoking a flow, I want one final JSON result so
that I can consume outputs without parsing headings or log previews.

**Acceptance Criteria:**

- [ ] `flow run <file|-> --json` runs in the foreground and emits exactly one
      final JSON document.
- [ ] JSON mode implies no editor and no detached background process.
- [ ] The final result includes `run_id`, flow summary, start and finish times,
      elapsed duration, success state, and every block's final output.
- [ ] Outputs are encoded without truncation in the final result.
- [ ] Output ordering is deterministic and follows source block order; callers
      are not exposed to Go map iteration order.
- [ ] `--output <directory>` may still write output files and the JSON result
      reports the directory and written paths.
- [ ] Execution failures emit the structured error/result document and exit 3.
- [ ] No progress prose, editor URL, or output headings appear on stdout.
- [ ] Existing text foreground and background modes remain available and
      unchanged by default.
- [ ] Runtime and E2E tests cover successful, branching, and failed executions.
- [ ] `go test -race ./...` passes.

### US-007: JSON Lines execution events

**Description:** As an agent monitoring a long flow, I want structured progress
events so that I can observe it without waiting for one final document.

**Acceptance Criteria:**

- [ ] `flow run <file|-> --jsonl` runs in the foreground without an editor and
      emits one valid JSON object per line.
- [ ] JSONL records cover run started, block started, block finished, run
      finished, and execution error events.
- [ ] Every record includes `schema_version`, `event`, `run_id`, `sequence`,
      and timestamp.
- [ ] The final record contains the complete deterministic output list or a
      structured terminal error.
- [ ] Event sequences are strictly increasing per run.
- [ ] `--json` and `--jsonl` are mutually exclusive and invalid combinations
      exit 2 with a structured error when either structured flag is active.
- [ ] The implementation adapts the runtime observer/event model through a
      stable CLI DTO rather than declaring the internal live event schema to be
      the permanent CLI contract.
- [ ] E2E tests decode every emitted line and verify event order and terminal
      state.
- [ ] `go test -race ./...` passes.

### US-008: JSON external inputs

**Description:** As an automation author, I want to supply all external inputs
as a JSON object so that multiline code and punctuation do not require shell
escaping.

**Acceptance Criteria:**

- [ ] `flow run ... --inputs-json <path|->` accepts a JSON object whose values
      are strings.
- [ ] Existing repeatable `--input name=value` and `--input name=@path` flags
      remain supported.
- [ ] Explicit `--input` values override the same keys loaded from
      `--inputs-json`; this precedence is documented and tested.
- [ ] Non-object JSON, non-string values, duplicate stdin ownership, unreadable
      files, and malformed JSON produce structured input errors.
- [ ] External input origin metadata distinguishes inline, file, and JSON
      sources without exposing secret values unnecessarily in diagnostics.
- [ ] E2E tests cover multiline values, precedence, malformed input, and stdin.
- [ ] `go test ./...` passes.

### US-009: Explicit editor control

**Description:** As a CLI user, I want to suppress browser/editor startup even
in text mode so that headless environments have no UI side effects.

**Acceptance Criteria:**

- [ ] `flow run ... --no-editor` executes without discovering, starting, or
      registering an editor.
- [ ] `--json`, `--jsonl`, and `--dry-run --json` imply `--no-editor`.
- [ ] Text-mode behavior without `--no-editor` remains unchanged.
- [ ] `--no-editor` is compatible with foreground execution.
- [ ] Background text execution with `--no-editor` is either supported with a
      documented process/log result or rejected clearly; the implementation
      must not silently change semantics.
- [ ] E2E tests use an isolated cache and prove no editor descriptors are
      created.
- [ ] `go test ./...` passes.

### US-010: Documentation and Flows skill update

**Description:** As a Codex user, I want the existing Flows skill to choose the
machine interface automatically so that agent workflows remain reliable and
concise.

**Acceptance Criteria:**

- [ ] README CLI documentation includes JSON validation, inspection, stdin,
      JSON inputs, JSON execution, JSONL execution, and `--no-editor` examples.
- [ ] The existing `.agents/skills/flows/SKILL.md` is updated; no second Flows
      skill is introduced.
- [ ] The skill tells agents to prefer JSON for bounded commands, JSONL when
      monitoring live execution, and text/chart output when communicating with
      a human.
- [ ] The skill preserves current authoring guidance, validation rules, goal
      card distinctions, and write-capable executor boundaries.
- [ ] Detailed CLI examples live in the existing conditional reference rather
      than unnecessarily expanding the skill entrypoint.
- [ ] The skill passes `quick_validate.py` from the system skill-creator.
- [ ] All command examples in the README and skill are exercised in tests or a
      documented smoke-test script.
- [ ] `go test ./...`, `go test -race ./...`, and `go vet ./...` pass.

## 4. Functional Requirements

1. **FR-001:** Text mode remains the default for all existing commands.
2. **FR-002:** `--json` selects a single-document JSON contract.
3. **FR-003:** `--jsonl` selects a streaming JSON Lines contract and is valid
   only for execution commands that emit progress.
4. **FR-004:** JSON and JSONL schemas begin at integer `schema_version: 1`.
5. **FR-005:** Structured stdout never contains banners, progress prose,
   browser URLs, warnings, or Cobra usage text.
6. **FR-006:** Structured failures are emitted before the process exits with a
   non-zero status.
7. **FR-007:** `-` consistently means stdin for flow source and JSON input,
   with exclusive ownership enforced.
8. **FR-008:** `flow inspect` exposes a public DTO independent of `model.Flow`.
9. **FR-009:** Machine execution is foreground-only in this release.
10. **FR-010:** Machine execution does not start, discover, or require the
    browser editor.
11. **FR-011:** Final outputs and inspected blocks use deterministic source
    order.
12. **FR-012:** All JSON is valid UTF-8 and encoded with Go's standard JSON
    encoder without terminal color codes.
13. **FR-013:** Input values remain strings because the runtime input contract
    is string-based.
14. **FR-014:** Existing environment overrides for executors, models, tokens,
    timeouts, and Python remain available in machine mode.
15. **FR-015:** Machine-facing schema types are documented with representative
    success and failure examples.

## 5. Non-Goals

1. Implementing an MCP server.
2. Creating a new Flows skill separate from the existing one.
3. Persisting runs or adding `flow status` and `flow cancel`; these require a
   durable run manager and are a later feature.
4. Turning the CLI into a daemon or remote service.
5. Adding a CLI command that edits or writes flow Markdown on behalf of the
   caller.
6. Building lossless structured source edits or an AST-preserving formatter.
7. Changing the flow Markdown schema.
8. Changing browser chart behavior outside explicit `--no-editor` execution.
9. Guaranteeing source line and column diagnostics when the current parser
   does not retain source spans.
10. Removing or redesigning existing human-readable output.

## 6. Design Considerations

### Human and machine modes

The feature must keep the current approachable human CLI. Structured modes are
opt-in, except that choosing one deliberately changes execution to foreground,
headless operation. This avoids hidden child processes and makes a tool call's
lifetime match the flow run's lifetime.

### Output schemas

Public JSON types should be explicit DTOs in a small package such as
`pkg/clioutput`. Directly serializing `model.Flow`, `runtime.RunResult`, or
`live.EventEnvelope` would accidentally make internal refactors breaking CLI
changes. Additive fields may be introduced within schema version 1; removals,
renames, or semantic changes require a new schema version.

### Errors

Structured errors should be useful to an agent but must not leak API keys,
tokens, full environment variables, or unnecessary external input values.
Plain stderr remains appropriate for catastrophic failures before output mode
can be determined, but normal parse, validation, usage, configuration, and
execution failures must use the selected structured format.

### Skill behavior

The existing Flows skill is the right home because it already covers creating,
editing, validating, running, debugging, and explaining flow documents. The
machine CLI is a better tool path inside the same workflow, not a distinct
domain. [Official OpenAI documentation](https://developers.openai.com/codex/skills)
describes skills as packages of instructions and optional resources for
reliable reusable workflows; updating the existing focused skill follows that
model.

## 7. Technical Considerations

1. Move root command construction out of `main()` so E2E and command tests can
   inject stdin/stdout/stderr buffers without always spawning a process.
2. Add a source loader shared by validate, inspect, viz, and run.
3. Add a shared result writer that produces text, JSON, or JSONL and centralize
   error-to-exit-code mapping.
4. Keep Cobra from printing usage or a second plain error after a structured
   error has already been emitted.
5. Extend `runtime.RunResult` or an adjacent execution result with `run_id` and
   timing data required by the final CLI contract.
6. Implement a CLI observer that converts live runtime events to versioned
   JSONL records without needing the editor HTTP observer.
7. Build output slices by walking `flow.Agents` rather than ranging over the
   runtime output map.
8. Treat editor startup as an optional adapter around execution, not a
   prerequisite inside the core run path.
9. Preserve existing `FLOW_*` environment behavior and current prompt routing.
10. Test stdout and stderr separately; `CombinedOutput` cannot prove the
    machine stdout purity requirement.
11. Run the existing JAX E2E test using the ignored `.venv` environment, plus
    the full race suite.
12. Validate the updated skill with:

    ```bash
    /home/sam/.codex/skills/.system/skill-creator/scripts/quick_validate.py \
      .agents/skills/flows
    ```

## 8. Success Metrics

1. Every structured-mode E2E test decodes stdout without filtering any lines.
2. Invalid flows and failed runs yield both a parseable structured error and
   the documented non-zero exit code.
3. Machine execution leaves no editor descriptor or detached run process.
4. Existing text-mode E2E tests pass without expectation changes except where
   explicitly approved by this PRD.
5. All example flows validate after implementation.
6. `go test ./...`, `go test -race ./...`, `go vet ./...`, and the skill
   validator all pass.
7. A fresh Codex session using the updated Flows skill can inspect, validate,
   dry-run, and run a flow using only structured CLI output.

## 9. Open Questions

1. Should a later version consolidate `--json` and `--jsonl` into a global
   `--format text|json|jsonl` flag after compatibility experience is gathered?
2. Should JSONL block-finished events contain complete outputs or bounded
   previews? This PRD requires complete outputs only in the final record; event
   payload limits should be chosen during implementation and documented.
3. Should a future durable run manager use the existing editor run store,
   replace it with a shared store, or introduce a separate execution service?
4. When Flows is distributed beyond this repository, should the existing skill
   move to the current `.agents/skills` convention or be packaged with a
   plugin? That packaging decision is outside this CLI feature.
