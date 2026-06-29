import type {
  EventEnvelope,
  LiveUIState,
  RunRecord,
  RunSnapshot,
} from "./types";

export const PROTOCOL_VERSION = 1;

export function emptyLiveState(): LiveUIState {
  return { runsById: {}, runOrder: [], selectedRunId: null };
}

export function applySnapshot(
  state: LiveUIState,
  snap: RunSnapshot
): LiveUIState {
  if (snap.version !== PROTOCOL_VERSION) return state;
  const runsById: Record<string, RunRecord> = {};
  const runOrder: string[] = [];
  for (const run of snap.runs ?? []) {
    runsById[run.run_id] = run;
    runOrder.push(run.run_id);
  }
  let selected = state.selectedRunId;
  if (!selected || !runsById[selected]) {
    selected = pickSelectedRun(runsById, runOrder);
  }
  return { runsById, runOrder, selectedRunId: selected };
}

function pickSelectedRun(
  runsById: Record<string, RunRecord>,
  runOrder: string[]
): string | null {
  for (let i = runOrder.length - 1; i >= 0; i--) {
    const r = runsById[runOrder[i]];
    if (r && !r.finished_at) return r.run_id;
  }
  for (let i = runOrder.length - 1; i >= 0; i--) {
    const r = runsById[runOrder[i]];
    if (r) return r.run_id;
  }
  return null;
}

export function applyEvent(
  state: LiveUIState,
  env: EventEnvelope
): LiveUIState {
  if (env.version !== PROTOCOL_VERSION) {
    console.warn(`live: ignoring event with unknown version ${env.version}`);
    return state;
  }
  const existing = state.runsById[env.run_id];
  if (existing && env.seq <= existing.last_seq) {
    return state;
  }
  if (existing && env.seq > existing.last_seq + 1) {
    console.warn(
      `live: gap in seq for run ${env.run_id} (${existing.last_seq} → ${env.seq})`
    );
  }

  const run: RunRecord = existing
    ? { ...existing, agents: { ...existing.agents } }
    : {
        run_id: env.run_id,
        started_at: env.ts,
        disconnected: false,
        last_seq: 0,
        agents: {},
      };
  run.last_seq = env.seq;

  switch (env.kind) {
    case "run_started":
      run.started_at = env.ts;
      run.external_inputs = env.external_inputs;
      break;
    case "agent_started":
      if (env.agent) {
        const prev = run.agents[env.agent];
        run.agents[env.agent] = {
          ...(prev ?? { iter: 0, status: "idle" }),
          status: "running",
          iter: env.iter ?? (prev?.iter ?? 0) + 1,
        };
      }
      break;
    case "agent_finished":
      if (env.agent) {
        const prev = run.agents[env.agent];
        run.agents[env.agent] = {
          ...(prev ?? { iter: 0, status: "idle" }),
          status: env.status === "failed" ? "failed" : "done",
          iter: env.iter ?? prev?.iter ?? 1,
          duration_ms: env.duration_ms,
          output_preview: env.output_preview,
          output_bytes: env.output_bytes,
          output_truncated: env.output_truncated,
          error: env.error,
        };
      }
      break;
    case "run_finished":
      run.finished_at = env.ts;
      run.ok = env.ok;
      run.error = env.error;
      break;
  }

  const runsById = { ...state.runsById, [env.run_id]: run };
  const runOrder = existing ? state.runOrder : [...state.runOrder, env.run_id];

  let selectedRunId = state.selectedRunId;
  if (!selectedRunId) {
    selectedRunId = env.run_id;
  } else if (!runsById[selectedRunId]) {
    selectedRunId = pickSelectedRun(runsById, runOrder);
  } else if (!existing) {
    const cur = runsById[selectedRunId];
    if (cur && cur.finished_at) selectedRunId = env.run_id;
  }

  return { runsById, runOrder, selectedRunId };
}
