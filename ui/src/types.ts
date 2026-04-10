export interface FlowJSON {
  name: string;
  description: string;
  external_inputs: string[];
  defaults: DefaultsJSON;
  agents: AgentJSON[];
}

export interface DefaultsJSON {
  model?: string;
  temperature?: number;
}

export interface AgentJSON {
  name: string;
  position: [number, number];
  inputs: Record<string, InputJSON>;
  start: ConditionJSON[];
  node_type: "prompt" | "function";
  language?: string;
  content: string;
  model?: string;
  temperature?: number;
  on_error?: string;
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
