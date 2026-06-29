export interface FlowJSON {
  name: string;
  description: string;
  external_inputs: string[];
  defaults: DefaultsJSON;
  agents: AgentJSON[];
}

export interface DefaultsJSON {
  prompt_executor?: string;
  model?: string;
  temperature?: number;
}

export interface AgentJSON {
  name: string;
  position: [number, number];
  inputs: Record<string, InputJSON>;
  start: ConditionJSON[];
  goal?: GoalJSON;
  node_type: "prompt" | "function";
  language?: string;
  content: string;
  prompt_executor?: string;
  model?: string;
  temperature?: number;
  on_error?: string;
  on_exhaustion?: string;
}

export interface GoalJSON {
  objective: string;
  validation?: string[];
  max_turns?: number;
  token_budget?: number;
  on_exhaustion?: string;
}

export interface InputJSON {
  from: string;
  fallback?: string;
}

export interface ConditionJSON {
  always?: { max_runs: number };
  when?: string[];
  contains?: string;
  max_runs?: number;
  on_exhaustion?: string;
}

export interface WSMessage {
  type: string;
  data: unknown;
}

// --- Live execution state ---

export type AgentLiveStatus = "idle" | "running" | "done" | "failed";

export interface AgentLiveState {
  status: AgentLiveStatus;
  iter: number;
  duration_ms?: number;
  output_preview?: string;
  output_bytes?: number;
  output_truncated?: boolean;
  error?: string;
}

export interface RunRecord {
  run_id: string;
  started_at: string;
  finished_at?: string;
  ok?: boolean;
  error?: string;
  disconnected: boolean;
  last_seq: number;
  agents: Record<string, AgentLiveState>;
  external_inputs?: ExternalInputOrigin[];
}

export interface ExternalInputOrigin {
  name: string;
  source: "inline" | "file";
  path?: string;
  file_name?: string;
  bytes?: number;
  preview?: string;
  preview_truncated?: boolean;
}

export interface RunSnapshot {
  version: number;
  flow_key: string;
  generated_at: string;
  runs: RunRecord[];
}

export type EventKind =
  | "run_started"
  | "agent_started"
  | "agent_finished"
  | "run_finished";

export interface EventEnvelope {
  version: number;
  kind: EventKind;
  flow_key: string;
  run_id: string;
  seq: number;
  ts: string;
  agent?: string;
  iter?: number;
  status?: "done" | "failed";
  duration_ms?: number;
  output_preview?: string;
  output_bytes?: number;
  output_truncated?: boolean;
  error?: string;
  ok?: boolean;
  external_inputs?: ExternalInputOrigin[];
}

export interface LiveUIState {
  runsById: Record<string, RunRecord>;
  runOrder: string[];
  selectedRunId: string | null;
}
