import { MarkerType, type Node, type Edge } from "@xyflow/react";
import type { ELK, ElkExtendedEdge, ElkNode, ElkPort } from "elkjs";
import type {
  FlowJSON,
  AgentJSON,
  AgentLiveState,
  ExternalInputOrigin,
} from "./types";

const GRID_SIZE = 100;
const AGENT_NODE_WIDTH = 280;
const AGENT_NODE_HEIGHT = 132;
const INPUT_NODE_WIDTH = 180;
const INPUT_NODE_HEIGHT = 42;
const OUTPUT_NODE_WIDTH = 170;
const OUTPUT_NODE_HEIGHT = 46;
const GOAL_NODE_WIDTH = 210;
const GOAL_NODE_HEIGHT = 104;
const TERMINAL_NODE_WIDTH = 116;
const TERMINAL_NODE_HEIGHT = 54;
const LOOP_FRAME_MIN_WIDTH = 360;
const LOOP_FRAME_MIN_HEIGHT = 260;
const PORT_SIZE = 8;
const TIME_RANK_SPACING = 820;
const INPUT_OFFSET_X = 260;
const OUTPUT_OFFSET_X = AGENT_NODE_WIDTH + 28;
const SIDECAR_GAP_Y = 18;
const GOAL_OFFSET_Y = GOAL_NODE_HEIGHT + 22;
const GOAL_CENTER_OFFSET_X = (AGENT_NODE_WIDTH - GOAL_NODE_WIDTH) / 2;
const LOOP_FRAME_PADDING_X = 92;
const LOOP_FRAME_PADDING_TOP = 86;
const LOOP_FRAME_PADDING_BOTTOM = 128;
const LOOP_FRAME_NESTED_INSET = 48;
const EXIT_LANE_GAP = 360;

const ROUTE_COLOR = "#111827";
const END_ROUTE_COLOR = "#475569";
const DATA_COLOR = "#0d9488";
const GOAL_COLOR = "#7c3aed";

interface LoopColor {
  stroke: string;
  fill: string;
  soft: string;
  badgeBg: string;
  text: string;
}

const LOOP_COLORS: LoopColor[] = [
  {
    stroke: "#2563eb",
    fill: "rgba(219, 234, 254, 0.42)",
    soft: "#dbeafe",
    badgeBg: "#dbeafe",
    text: "#1d4ed8",
  },
  {
    stroke: "#d97706",
    fill: "rgba(254, 243, 199, 0.44)",
    soft: "#fde68a",
    badgeBg: "#fef3c7",
    text: "#92400e",
  },
  {
    stroke: "#059669",
    fill: "rgba(209, 250, 229, 0.42)",
    soft: "#a7f3d0",
    badgeBg: "#d1fae5",
    text: "#047857",
  },
  {
    stroke: "#7c3aed",
    fill: "rgba(237, 233, 254, 0.42)",
    soft: "#ddd6fe",
    badgeBg: "#ede9fe",
    text: "#6d28d9",
  },
  {
    stroke: "#e11d48",
    fill: "rgba(255, 228, 230, 0.42)",
    soft: "#fecdd3",
    badgeBg: "#ffe4e6",
    text: "#be123c",
  },
  {
    stroke: "#0891b2",
    fill: "rgba(207, 250, 254, 0.42)",
    soft: "#a5f3fc",
    badgeBg: "#cffafe",
    text: "#0e7490",
  },
];

let elkPromise: Promise<ELK> | null = null;

export const START_NODE_ID = "__flow:start";
export const END_NODE_ID = "__flow:end";
export const loopFrameNodeId = (source: string, target: string) =>
  `loop:${source}:${target}`;
export const externalNodeId = (name: string) => `external:${name}`;
export const inputNodeId = (agent: string, input: string) =>
  `input:${agent}:${input}`;
export const outputNodeId = (
  source: string,
  target: string,
  input: string,
  isFallback: boolean
) => `output:${source}:${target}:${input}${isFallback ? ":fallback" : ""}`;
export const triggerOutputNodeId = (source: string, target: string) =>
  `trigger-output:${source}:${target}`;
export const goalNodeId = (agent: string) => `goal:${agent}`;

function labelLetter(agent: AgentJSON): string {
  if (agent.node_type === "prompt") return "A";
  const lang = agent.language ?? "";
  return (lang[0] ?? "F").toUpperCase();
}

export function computeLabels(flow: FlowJSON): Map<string, string> {
  const labels = new Map<string, string>();
  const counters = new Map<string, number>();
  for (const agent of flow.agents) {
    const letter = labelLetter(agent);
    const next = (counters.get(letter) ?? 0) + 1;
    counters.set(letter, next);
    labels.set(agent.name, `${letter}:${next}`);
  }
  return labels;
}

export function flowToNodes(
  flow: FlowJSON,
  agentLive?: Record<string, AgentLiveState>
): Node[] {
  const labels = computeLabels(flow);
  const roles = computeGraphRoles(flow);
  const connections = computeConnectionModel(flow);
  const ranks = computeExecutionRanks(flow);
  const loops = computeFeedbackLoops(ranks, connections);
  const loopByKey = new Map(loops.map((loop) => [loop.key, loop] as const));

  const agentNodes: Node[] = flow.agents.map((agent) => ({
    id: agent.name,
    type: agent.node_type === "function" ? "functionNode" : "agentNode",
    zIndex: 20,
    position: {
      x: agent.position[0] * GRID_SIZE,
      y: agent.position[1] * GRID_SIZE,
    },
    data: {
      agent,
      label: labels.get(agent.name) ?? "",
      liveState: agentLive?.[agent.name],
      role: roles.get(agent.name),
      inputs: summarizeInputs(agent, labels),
      defaultModel: flow.defaults.model,
    },
  }));

  const terminalNodes: Node[] = [
    {
      id: START_NODE_ID,
      type: "terminalNode",
      zIndex: 20,
      position: { x: 0, y: 0 },
      data: {
        kind: "terminal",
        terminalKind: "start",
        label: "START",
        description: "Flow entry",
      },
    },
    {
      id: END_NODE_ID,
      type: "terminalNode",
      zIndex: 20,
      position: {
        x:
          (Math.max(0, ...Array.from(ranks.values())) + 2) *
          TIME_RANK_SPACING,
        y: 0,
      },
      data: {
        kind: "terminal",
        terminalKind: "end",
        label: "END",
        description: "Flow exit",
      },
    },
  ];

  const loopFrameNodes: Node[] = loops.map((loop) => ({
    id: loop.id,
    type: "loopFrameNode",
    position: { x: 0, y: 0 },
    selectable: false,
    draggable: false,
    focusable: false,
    zIndex: -10 + loop.depth,
    style: { pointerEvents: "none" },
    data: {
      kind: "loopFrame",
      label: "LOOP",
      condition: loop.label,
      source: loop.source,
      target: loop.target,
      depth: loop.depth,
      loopColor: loop.color,
      width: LOOP_FRAME_MIN_WIDTH,
      height: LOOP_FRAME_MIN_HEIGHT,
    },
  }));

  const inputNodes: Node[] = flow.agents.flatMap((agent) =>
    Object.entries(agent.inputs).map(([inputName, input], i) => ({
      id: inputNodeId(agent.name, inputName),
      type: "inputNode",
      zIndex: 20,
      position: {
        x: (agent.position[0] - 1) * GRID_SIZE,
        y:
          agent.position[1] * GRID_SIZE +
          AGENT_NODE_HEIGHT +
          SIDECAR_GAP_Y +
          i * (INPUT_NODE_HEIGHT + 12),
      },
      data: {
        kind: "input",
        name: inputName,
        target: agent.name,
        source: sourceLabel(input.from),
        sourceKind: sourceKind(input.from),
        sourceBlockLabel: blockLabel(input.from, labels),
        fallback: input.fallback ? sourceLabel(input.fallback) : undefined,
        fallbackKind: input.fallback ? sourceKind(input.fallback) : undefined,
        fallbackBlockLabel: input.fallback
          ? blockLabel(input.fallback, labels)
          : undefined,
      },
    }))
  );

  const goalNodes: Node[] = flow.agents
    .filter((agent) => agent.goal)
    .map((agent) => ({
      id: goalNodeId(agent.name),
      type: "goalNode",
      zIndex: 20,
      position: {
        x: agent.position[0] * GRID_SIZE + GOAL_CENTER_OFFSET_X,
        y: agent.position[1] * GRID_SIZE - GOAL_OFFSET_Y,
      },
      data: {
        kind: "goal",
        target: agent.name,
        objective: agent.goal?.objective ?? "",
        validation: agent.goal?.validation,
        maxTurns: agent.goal?.max_turns,
        tokenBudget: agent.goal?.token_budget,
        onExhaustion: agent.goal?.on_exhaustion,
      },
    }));

  const outputNodes: Node[] = connections.dataConnections.map((conn) => {
    const key = edgeKey(conn.source, conn.target);
    const isFeedback = isFeedbackEdge(ranks, conn.source, conn.target);
    return {
      id: outputNodeId(conn.source, conn.target, conn.inputName, conn.isFallback),
      type: "outputNode",
      zIndex: 20,
      position: { x: 0, y: 0 },
      data: {
        kind: "output",
        name: conn.label,
        source: conn.source,
        sourceBlockLabel: labels.get(conn.source),
        target: conn.target,
        targetBlockLabel: labels.get(conn.target),
        inputName: conn.inputName,
        isFallback: conn.isFallback,
        isFeedback,
        loopColor: loopByKey.get(key)?.color,
        condition: conditionSummary(
          connections.triggerSpecs.get(key),
          isFeedback
        ),
      },
    };
  });

  const triggerOutputNodes: Node[] = connections.triggerOnly.map((trigger) => {
    const isFeedback = isFeedbackEdge(ranks, trigger.source, trigger.target);
    return {
      id: triggerOutputNodeId(trigger.source, trigger.target),
      type: "outputNode",
      zIndex: 20,
      position: { x: 0, y: 0 },
      data: {
        kind: "output",
        name: "trigger",
        source: trigger.source,
        sourceBlockLabel: labels.get(trigger.source),
        target: trigger.target,
        targetBlockLabel: labels.get(trigger.target),
        isFeedback,
        loopColor: loopByKey.get(trigger.key)?.color,
        condition: conditionSummary(trigger, isFeedback),
      },
    };
  });

  return [
    ...loopFrameNodes,
    ...terminalNodes,
    ...agentNodes,
    ...inputNodes,
    ...goalNodes,
    ...outputNodes,
    ...triggerOutputNodes,
  ];
}

// decorateNodes returns a copy of nodes with each node's liveState updated
// from the given agent map. Used to refresh nodes in place when live events
// arrive, without rebuilding from scratch.
export function decorateNodes(
  nodes: Node[],
  agentLive?: Record<string, AgentLiveState>,
  externalInputs?: ExternalInputOrigin[]
): Node[] {
  const externalOrigins = new Map(
    (externalInputs ?? []).map((origin) => [origin.name, origin])
  );
  return nodes.map((n) => ({
    ...n,
    data: decorateNodeData(n, agentLive, externalOrigins),
  }));
}

function decorateNodeData(
  node: Node,
  agentLive?: Record<string, AgentLiveState>,
  externalOrigins?: Map<string, ExternalInputOrigin>
) {
  const data = node.data as Record<string, unknown>;
  return {
    ...data,
    liveState: agentLive?.[node.id],
    externalOrigin:
      data.kind === "input"
        ? externalOrigins?.get(String(data.name ?? ""))
        : undefined,
  };
}

export function flowToEdges(flow: FlowJSON): Edge[] {
  const edges: Edge[] = [];
  const ranks = computeExecutionRanks(flow);
  const roles = computeGraphRoles(flow);
  const connections = computeConnectionModel(flow);
  const loops = computeFeedbackLoops(ranks, connections);
  const loopKeys = new Set(loops.map((loop) => loop.key));
  const loopByKey = new Map(loops.map((loop) => [loop.key, loop] as const));

  for (const agent of flow.agents) {
    const always = agent.start.find((cond) => !!cond.always)?.always;
    if (always) {
      edges.push(startToBlockEdge(agent.name, always.max_runs));
    }
    if (agent.goal) {
      edges.push(goalToBlockEdge(agent.name));
    }
  }

  for (const agent of flow.agents) {
    for (const inputName of Object.keys(agent.inputs)) {
      edges.push(inputToBlockEdge(agent.name, inputName));
    }
  }

  for (const conn of connections.dataConnections) {
    const key = edgeKey(conn.source, conn.target);
    const trigger = connections.triggerSpecs.get(key);
    const isFeedback = isFeedbackEdge(ranks, conn.source, conn.target);
    const loopColor = loopByKey.get(key)?.color;
    edges.push(sourceToOutputEdge(conn, trigger, isFeedback, loopColor));
    if (loopKeys.has(key)) {
      continue;
    }
    edges.push(outputToInputEdge(conn, trigger, isFeedback, loopColor));
  }

  for (const spec of connections.triggerOnly) {
    const isFeedback = isFeedbackEdge(ranks, spec.source, spec.target);
    const loopColor = loopByKey.get(spec.key)?.color;
    edges.push(sourceToTriggerOutputEdge(spec, isFeedback, loopColor));
    if (loopKeys.has(edgeKey(spec.source, spec.target))) {
      continue;
    }
    edges.push(triggerOutputToBlockEdge(spec, isFeedback, loopColor));
  }

  const endSources = flow.agents
    .filter((agent) => {
      const role = roles.get(agent.name);
      return Boolean(role?.isFinish || role?.isStopGate);
    })
    .sort((a, b) => {
      const rankA = ranks.get(a.name) ?? 0;
      const rankB = ranks.get(b.name) ?? 0;
      if (rankA !== rankB) return rankA - rankB;
      return a.name.localeCompare(b.name);
    });

  endSources.forEach((agent) => {
    const role = roles.get(agent.name);
    edges.push(blockToEndEdge(agent.name, Boolean(role?.isStopGate)));
  });

  return edges;
}

interface InputSummary {
  name: string;
  source: string;
  sourceKind: "agent" | "external";
  sourceBlockLabel?: string;
  fallback?: string;
  fallbackKind?: "agent" | "external";
  fallbackBlockLabel?: string;
}

interface AgentRole {
  isStart: boolean;
  isFinish: boolean;
  isStopGate: boolean;
}

interface ControlStats {
  total: number;
  unconditional: number;
  conditional: number;
}

interface DataConnectionSpec {
  source: string;
  target: string;
  inputName: string;
  label: string;
  isFallback: boolean;
}

interface TriggerSpec {
  source: string;
  target: string;
  key: string;
  rules: TriggerRule[];
  isConditional: boolean;
}

interface TriggerRule {
  contains?: string;
  maxRuns?: number;
}

interface LoopSpec {
  id: string;
  key: string;
  source: string;
  target: string;
  label: string;
  startRank: number;
  endRank: number;
  depth: number;
  color: LoopColor;
  body: Set<string>;
}

interface AgentEdgeSpec {
  source: string;
  target: string;
  key: string;
}

interface ConnectionModel {
  dataConnections: DataConnectionSpec[];
  triggerSpecs: Map<string, TriggerSpec>;
  triggerOnly: TriggerSpec[];
}

function summarizeInputs(
  agent: AgentJSON,
  labels: Map<string, string>
): InputSummary[] {
  return Object.entries(agent.inputs).map(([name, input]) => ({
    name,
    source: sourceLabel(input.from),
    sourceKind: sourceKind(input.from),
    sourceBlockLabel: blockLabel(input.from, labels),
    fallback: input.fallback ? sourceLabel(input.fallback) : undefined,
    fallbackKind: input.fallback ? sourceKind(input.fallback) : undefined,
    fallbackBlockLabel: input.fallback
      ? blockLabel(input.fallback, labels)
      : undefined,
  }));
}

function sourceLabel(source: string): string {
  return source === "external" ? "external" : source;
}

function blockLabel(source: string, labels: Map<string, string>): string | undefined {
  return source === "external" ? undefined : labels.get(source);
}

function sourceKind(source: string): "agent" | "external" {
  return source === "external" ? "external" : "agent";
}

function computeGraphRoles(flow: FlowJSON): Map<string, AgentRole> {
  const agentNames = new Set(flow.agents.map((agent) => agent.name));
  const outgoing = new Map<string, ControlStats>();
  const starts = new Set<string>();

  for (const agent of flow.agents) {
    outgoing.set(agent.name, { total: 0, unconditional: 0, conditional: 0 });
    if (agent.start.some((cond) => !!cond.always)) {
      starts.add(agent.name);
    }
  }

  for (const agent of flow.agents) {
    for (const cond of agent.start) {
      if (!cond.when) continue;
      const isConditional = Boolean(cond.contains || cond.max_runs);
      for (const dep of cond.when) {
        if (!agentNames.has(dep)) continue;
        const stats = outgoing.get(dep);
        if (!stats) continue;
        stats.total += 1;
        if (isConditional) {
          stats.conditional += 1;
        } else {
          stats.unconditional += 1;
        }
      }
    }
  }

  const roles = new Map<string, AgentRole>();
  for (const agent of flow.agents) {
    const stats = outgoing.get(agent.name) ?? {
      total: 0,
      unconditional: 0,
      conditional: 0,
    };
    roles.set(agent.name, {
      isStart: starts.has(agent.name),
      isFinish: stats.total === 0,
      isStopGate: stats.total > 0 && stats.unconditional === 0,
    });
  }
  return roles;
}

function computeExecutionRanks(flow: FlowJSON): Map<string, number> {
  const connections = computeConnectionModel(flow);
  const ranks = computeBaseExecutionRanks(flow);
  const loops = computeFeedbackLoops(ranks, connections);
  return adjustRanksForLoopExits(ranks, loops, agentEdges(connections));
}

function computeBaseExecutionRanks(flow: FlowJSON): Map<string, number> {
  const ranks = new Map<string, number>();
  for (const agent of flow.agents) {
    if (agent.start.some((cond) => !!cond.always)) {
      ranks.set(agent.name, 0);
    }
  }

  let changed = true;
  while (changed) {
    changed = false;
    for (const agent of flow.agents) {
      if (ranks.has(agent.name)) continue;
      let nextRank: number | undefined;
      for (const cond of agent.start) {
        for (const dep of cond.when ?? []) {
          const depRank = ranks.get(dep);
          if (depRank === undefined) continue;
          nextRank = Math.max(nextRank ?? 0, depRank + 1);
        }
      }
      if (nextRank !== undefined) {
        ranks.set(agent.name, nextRank);
        changed = true;
      }
    }
  }

  const fallbackRank =
    Math.max(0, ...Array.from(ranks.values())) + 1;
  for (const agent of flow.agents) {
    if (!ranks.has(agent.name)) {
      ranks.set(agent.name, fallbackRank);
    }
  }
  return ranks;
}

function adjustRanksForLoopExits(
  baseRanks: Map<string, number>,
  loops: LoopSpec[],
  edges: AgentEdgeSpec[]
): Map<string, number> {
  const ranks = new Map(baseRanks);
  const loopKeys = new Set(loops.map((loop) => loop.key));

  let changed = true;
  while (changed) {
    changed = false;

    for (const loop of loops) {
      const loopEnd = maxRankForBody(ranks, loop.body);
      for (const edge of edges) {
        if (edge.key === loop.key) continue;
        if (!loop.body.has(edge.source) || loop.body.has(edge.target)) continue;
        const targetRank = ranks.get(edge.target) ?? 0;
        if (targetRank <= loopEnd + 1) {
          ranks.set(edge.target, loopEnd + 2);
          changed = true;
        }
      }
    }

    for (const edge of edges) {
      if (loopKeys.has(edge.key)) continue;
      if (isFeedbackEdge(baseRanks, edge.source, edge.target)) continue;
      const sourceRank = ranks.get(edge.source);
      if (sourceRank === undefined) continue;
      const targetRank = ranks.get(edge.target) ?? 0;
      if (targetRank <= sourceRank) {
        ranks.set(edge.target, sourceRank + 1);
        changed = true;
      }
    }
  }

  return ranks;
}

function maxRankForBody(ranks: Map<string, number>, body: Set<string>): number {
  return Math.max(
    0,
    ...Array.from(body).map((name) => ranks.get(name) ?? 0)
  );
}

function isFeedbackEdge(
  ranks: Map<string, number>,
  source: string,
  target: string
): boolean {
  const sourceRank = ranks.get(source);
  const targetRank = ranks.get(target);
  return (
    sourceRank !== undefined &&
    targetRank !== undefined &&
    sourceRank >= targetRank
  );
}

function computeFeedbackLoops(
  ranks: Map<string, number>,
  connections: ConnectionModel
): LoopSpec[] {
  const loops = new Map<string, LoopSpec>();
  const edges = agentEdges(connections);

  const addLoop = (
    source: string,
    target: string,
    trigger?: TriggerSpec
  ): void => {
    if (!isFeedbackEdge(ranks, source, target)) return;
    const startRank = ranks.get(target);
    const endRank = ranks.get(source);
    if (startRank === undefined || endRank === undefined) return;

    const key = edgeKey(source, target);
    const label = trigger
      ? formatTriggerLabel(trigger, true)
      : `LOOP: repeat to ${target}`;
    const body = computeLoopBody(source, target, startRank, endRank, ranks, edges);
    loops.set(key, {
      id: loopFrameNodeId(source, target),
      key,
      source,
      target,
      label,
      startRank,
      endRank,
      depth: 0,
      color: LOOP_COLORS[0],
      body,
    });
  };

  for (const trigger of connections.triggerSpecs.values()) {
    addLoop(trigger.source, trigger.target, trigger);
  }

  for (const conn of connections.dataConnections) {
    addLoop(
      conn.source,
      conn.target,
      connections.triggerSpecs.get(edgeKey(conn.source, conn.target))
    );
  }

  const sorted = Array.from(loops.values()).sort((a, b) => {
    if (a.startRank !== b.startRank) return a.startRank - b.startRank;
    if (a.endRank !== b.endRank) return b.endRank - a.endRank;
    return a.key.localeCompare(b.key);
  });

  return sorted.map((loop, index) => ({
    ...loop,
    depth: sorted.filter((other) => loopContains(other, loop)).length,
    color: LOOP_COLORS[index % LOOP_COLORS.length],
  }));
}

function loopContains(container: LoopSpec, loop: LoopSpec): boolean {
  if (container === loop) return false;
  const intervalContains =
    container.startRank <= loop.startRank &&
    container.endRank >= loop.endRank &&
    (container.startRank < loop.startRank ||
      container.endRank > loop.endRank);
  return intervalContains || isStrictSuperset(container.body, loop.body);
}

function computeLoopBody(
  source: string,
  target: string,
  startRank: number,
  endRank: number,
  ranks: Map<string, number>,
  edges: AgentEdgeSpec[]
): Set<string> {
  const ownKey = edgeKey(source, target);
  const bodyEdges = edges.filter((edge) => {
    if (edge.key === ownKey) return false;
    if (!isFeedbackEdge(ranks, edge.source, edge.target)) return true;
    return feedbackEdgeInsideLoop(edge, startRank, endRank, ranks);
  });
  const forward = adjacency(bodyEdges, "forward");
  const reverse = adjacency(bodyEdges, "reverse");
  const reachableFromTarget = reachable(target, forward);
  const canReachSource = reachable(source, reverse);
  const body = new Set<string>([source, target]);

  for (const name of reachableFromTarget) {
    if (canReachSource.has(name)) {
      body.add(name);
    }
  }

  return body;
}

function feedbackEdgeInsideLoop(
  edge: AgentEdgeSpec,
  startRank: number,
  endRank: number,
  ranks: Map<string, number>
): boolean {
  const sourceRank = ranks.get(edge.source);
  const targetRank = ranks.get(edge.target);
  return (
    sourceRank !== undefined &&
    targetRank !== undefined &&
    targetRank >= startRank &&
    sourceRank <= endRank
  );
}

function agentEdges(connections: ConnectionModel): AgentEdgeSpec[] {
  const edges = new Map<string, AgentEdgeSpec>();
  for (const trigger of connections.triggerSpecs.values()) {
    edges.set(trigger.key, {
      source: trigger.source,
      target: trigger.target,
      key: trigger.key,
    });
  }
  for (const conn of connections.dataConnections) {
    const key = edgeKey(conn.source, conn.target);
    edges.set(key, {
      source: conn.source,
      target: conn.target,
      key,
    });
  }
  return Array.from(edges.values());
}

function adjacency(
  edges: AgentEdgeSpec[],
  direction: "forward" | "reverse"
): Map<string, string[]> {
  const result = new Map<string, string[]>();
  for (const edge of edges) {
    const from = direction === "forward" ? edge.source : edge.target;
    const to = direction === "forward" ? edge.target : edge.source;
    const list = result.get(from) ?? [];
    list.push(to);
    result.set(from, list);
  }
  return result;
}

function reachable(start: string, graph: Map<string, string[]>): Set<string> {
  const seen = new Set<string>();
  const stack = [start];
  while (stack.length > 0) {
    const current = stack.pop();
    if (!current || seen.has(current)) continue;
    seen.add(current);
    for (const next of graph.get(current) ?? []) {
      if (!seen.has(next)) stack.push(next);
    }
  }
  return seen;
}

function isStrictSuperset(a: Set<string>, b: Set<string>): boolean {
  if (a.size <= b.size) return false;
  for (const item of b) {
    if (!a.has(item)) return false;
  }
  return true;
}

function computeConnectionModel(flow: FlowJSON): ConnectionModel {
  const dataConnections: DataConnectionSpec[] = [];
  const triggerSpecs = new Map<string, TriggerSpec>();

  for (const agent of flow.agents) {
    for (const cond of agent.start) {
      if (!cond.when) continue;
      for (const dep of cond.when) {
        addTriggerSpec(triggerSpecs, dep, agent.name, cond);
      }
    }

    for (const [inputName, input] of Object.entries(agent.inputs)) {
      addDataConnection(
        dataConnections,
        agent.name,
        inputName,
        inputName,
        input.from,
        false
      );
      addDataConnection(
        dataConnections,
        agent.name,
        inputName,
        `${inputName} fallback`,
        input.fallback,
        true
      );
    }
  }

  const dataPairs = new Set(
    dataConnections.map((conn) => edgeKey(conn.source, conn.target))
  );
  const triggerOnly = Array.from(triggerSpecs.values()).filter(
    (trigger) => !dataPairs.has(edgeKey(trigger.source, trigger.target))
  );

  return { dataConnections, triggerSpecs, triggerOnly };
}

function addDataConnection(
  dataConnections: DataConnectionSpec[],
  target: string,
  inputName: string,
  label: string,
  source: string | undefined,
  isFallback: boolean
): void {
  if (!source || source === "external") return;
  dataConnections.push({ source, target, inputName, label, isFallback });
}

function addTriggerSpec(
  triggerEdges: Map<string, TriggerSpec>,
  source: string,
  target: string,
  cond: AgentJSON["start"][number]
): void {
  const key = edgeKey(source, target);
  const existing = triggerEdges.get(key);
  const rule = conditionRule(cond);
  if (existing) {
    existing.rules.push(rule);
    existing.isConditional =
      existing.isConditional || Boolean(cond.contains || cond.max_runs);
  } else {
    triggerEdges.set(key, {
      source,
      target,
      key,
      rules: [rule],
      isConditional: Boolean(cond.contains || cond.max_runs),
    });
  }
}

function conditionRule(cond: AgentJSON["start"][number]): TriggerRule {
  return {
    contains: cond.contains || undefined,
    maxRuns: cond.max_runs || undefined,
  };
}

function edgeKey(source: string, target: string): string {
  return `${source}->${target}`;
}

function conditionSummary(
  trigger: TriggerSpec | undefined,
  isFeedback: boolean
): string | undefined {
  if (!trigger) return undefined;
  return formatTriggerLabel(trigger, isFeedback) || undefined;
}

function feedbackColor(loopColor?: LoopColor): LoopColor {
  return loopColor ?? LOOP_COLORS[0];
}

function startToBlockEdge(target: string, maxRuns: number): Edge {
  const label = maxRuns > 1 ? `start, up to ${maxRuns} runs` : "start";
  return {
    id: `${START_NODE_ID}->${target}`,
    type: "smoothstep",
    source: START_NODE_ID,
    target,
    sourceHandle: "ctrl-out",
    targetHandle: "ctrl-in",
    label,
    interactionWidth: 24,
    pathOptions: { offset: 38, borderRadius: 12 },
    labelStyle: { fill: ROUTE_COLOR, fontSize: 11, fontWeight: 800 },
    labelBgStyle: { fill: "#ffffff", fillOpacity: 0.95 },
    style: {
      strokeWidth: 4.2,
      stroke: ROUTE_COLOR,
    },
    markerEnd: { type: MarkerType.ArrowClosed, color: ROUTE_COLOR },
    zIndex: 0,
    data: { isRoute: true, isStartRoute: true },
  } as Edge;
}

function blockToEndEdge(
  source: string,
  isStopGate: boolean
): Edge {
  const label = isStopGate ? "otherwise END" : "END";
  return {
    id: `${source}->${END_NODE_ID}`,
    type: "exitRoute",
    source,
    target: END_NODE_ID,
    sourceHandle: "ctrl-out",
    targetHandle: "ctrl-in",
    label,
    interactionWidth: 24,
    labelStyle: {
      fill: END_ROUTE_COLOR,
      fontSize: 10,
      fontWeight: 800,
    },
    labelBgStyle: {
      fill: "#ffffff",
      fillOpacity: 0.95,
    },
    style: {
      strokeWidth: 4,
      stroke: END_ROUTE_COLOR,
    },
    markerEnd: { type: MarkerType.ArrowClosed, color: END_ROUTE_COLOR },
    zIndex: 0,
    data: { isRoute: true, isEndRoute: true },
  } as Edge;
}

function inputToBlockEdge(target: string, inputName: string): Edge {
  return {
    id: `input:${target}:${inputName}->${target}`,
    type: "smoothstep",
    source: inputNodeId(target, inputName),
    target,
    sourceHandle: "data-out",
    targetHandle: "data-in",
    interactionWidth: 14,
    pathOptions: { offset: 18, borderRadius: 8 },
    style: {
      strokeWidth: 1.3,
      stroke: "#94a3b8",
    },
    markerEnd: { type: MarkerType.Arrow, color: "#94a3b8" },
    zIndex: 0,
    data: { isLocalInput: true },
  } as Edge;
}

function goalToBlockEdge(target: string): Edge {
  return {
    id: `${goalNodeId(target)}<->${target}`,
    type: "straight",
    source: goalNodeId(target),
    target,
    sourceHandle: "goal-out",
    targetHandle: "goal-in",
    interactionWidth: 12,
    style: {
      strokeWidth: 1.8,
      stroke: GOAL_COLOR,
    },
    markerStart: { type: MarkerType.Arrow, color: GOAL_COLOR },
    markerEnd: { type: MarkerType.Arrow, color: GOAL_COLOR },
    zIndex: 2,
    data: { isGoalLink: true },
  } as Edge;
}

function sourceToOutputEdge(
  spec: DataConnectionSpec,
  trigger: TriggerSpec | undefined,
  isFeedback: boolean,
  loopColor?: LoopColor
): Edge {
  const isRoute = Boolean(trigger);
  const loopTone = feedbackColor(loopColor);
  const color = isFeedback ? loopTone.stroke : isRoute ? ROUTE_COLOR : DATA_COLOR;
  return {
    id: `${spec.source}->${outputNodeId(
      spec.source,
      spec.target,
      spec.inputName,
      spec.isFallback
    )}`,
    type: "smoothstep",
    source: spec.source,
    target: outputNodeId(
      spec.source,
      spec.target,
      spec.inputName,
      spec.isFallback
    ),
    sourceHandle: "data-out",
    targetHandle: "data-in",
    animated: false,
    interactionWidth: isRoute || isFeedback ? 24 : 16,
    pathOptions: { offset: isFeedback ? 48 : 28, borderRadius: 10 },
    style: {
      strokeWidth: isFeedback ? 4.3 : isRoute ? 4 : 1.6,
      stroke: color,
      strokeDasharray: isRoute || isFeedback ? undefined : "5 4",
    },
    markerEnd: {
      type: isRoute || isFeedback ? MarkerType.ArrowClosed : MarkerType.Arrow,
      color,
    },
    zIndex: isRoute || isFeedback ? 1 : 0,
    data: { isFeedback, isOutput: true, isRoute },
  } as Edge;
}

function outputToInputEdge(
  spec: DataConnectionSpec,
  trigger: TriggerSpec | undefined,
  isFeedback: boolean,
  loopColor?: LoopColor
): Edge {
  const isRoute = Boolean(trigger);
  const loopTone = feedbackColor(loopColor);
  const color = isFeedback ? loopTone.stroke : isRoute ? ROUTE_COLOR : DATA_COLOR;
  return {
    id: `${outputNodeId(
      spec.source,
      spec.target,
      spec.inputName,
      spec.isFallback
    )}->${inputNodeId(spec.target, spec.inputName)}`,
    type: "smoothstep",
    source: outputNodeId(
      spec.source,
      spec.target,
      spec.inputName,
      spec.isFallback
    ),
    target: inputNodeId(spec.target, spec.inputName),
    sourceHandle: "data-out",
    targetHandle: "data-in",
    animated: false,
    interactionWidth: isRoute || isFeedback ? 24 : 18,
    pathOptions: { offset: isFeedback ? 58 : 28, borderRadius: 10 },
    style: {
      strokeWidth: isFeedback ? 4.4 : isRoute ? 4.1 : 1.7,
      stroke: color,
      strokeDasharray: isRoute || isFeedback ? undefined : "5 4",
    },
    markerEnd: {
      type: isRoute || isFeedback ? MarkerType.ArrowClosed : MarkerType.Arrow,
      color,
    },
    zIndex: isRoute || isFeedback ? 1 : 0,
    data: { isFeedback, isOutput: true, isRoute },
  } as Edge;
}

function sourceToTriggerOutputEdge(
  spec: TriggerSpec,
  isFeedback: boolean,
  loopColor?: LoopColor
): Edge {
  const color = isFeedback ? feedbackColor(loopColor).stroke : ROUTE_COLOR;
  return {
    id: `${spec.source}->${triggerOutputNodeId(spec.source, spec.target)}`,
    type: "smoothstep",
    source: spec.source,
    target: triggerOutputNodeId(spec.source, spec.target),
    sourceHandle: "ctrl-out",
    targetHandle: "data-in",
    animated: !isFeedback && spec.isConditional,
    interactionWidth: 24,
    pathOptions: { offset: isFeedback ? 58 : 34, borderRadius: 10 },
    style: {
      strokeWidth: isFeedback ? 4.4 : 4,
      stroke: color,
    },
    markerEnd: { type: MarkerType.ArrowClosed, color },
    zIndex: 0,
    data: { isFeedback, isTrigger: true, isRoute: true },
  } as Edge;
}

function triggerOutputToBlockEdge(
  spec: TriggerSpec,
  isFeedback: boolean,
  loopColor?: LoopColor
): Edge {
  const loopTone = feedbackColor(loopColor);
  const color = isFeedback ? loopTone.stroke : ROUTE_COLOR;
  return {
    id: `${triggerOutputNodeId(spec.source, spec.target)}->${spec.target}`,
    type: "smoothstep",
    source: triggerOutputNodeId(spec.source, spec.target),
    target: spec.target,
    sourceHandle: "ctrl-out",
    targetHandle: "ctrl-in",
    animated: !isFeedback && spec.isConditional,
    interactionWidth: 24,
    pathOptions: { offset: isFeedback ? 58 : 34, borderRadius: 10 },
    style: {
      strokeWidth: isFeedback ? 4.5 : 4.1,
      stroke: color,
    },
    markerEnd: { type: MarkerType.ArrowClosed, color },
    zIndex: 0,
    data: { isFeedback, isTrigger: true, isRoute: true },
  } as Edge;
}

function formatTriggerLabel(spec: TriggerSpec, isFeedback: boolean): string {
  const labels = spec.rules
    .map((rule) => formatTriggerRule(rule, isFeedback))
    .filter(Boolean);
  if (isFeedback) {
    return `LOOP: ${labels.length > 0 ? labels.join(" / ") : "repeat"}`;
  }
  if (labels.length === 0) return "";
  return labels.join(" / ");
}

function formatTriggerRule(rule: TriggerRule, isFeedback: boolean): string {
  const contains = rule.contains ? `output has "${rule.contains}"` : "";
  const maxRuns = rule.maxRuns
    ? isFeedback
      ? `up to ${rule.maxRuns} repeats`
      : `up to ${rule.maxRuns} runs`
    : "";
  if (contains && maxRuns) return `if ${contains}, ${maxRuns}`;
  if (contains) return `if ${contains}`;
  return maxRuns;
}

export async function autoLayout(
  flow: FlowJSON,
  nodes: Node[],
  edges: Edge[]
): Promise<Node[]> {
  const nodeIds = new Set(nodes.map((node) => node.id));
  const graph: ElkNode = {
    id: "root",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "RIGHT",
      "elk.edgeRouting": "ORTHOGONAL",
      "elk.spacing.nodeNode": "72",
      "elk.spacing.edgeEdge": "24",
      "elk.spacing.edgeNode": "32",
      "elk.spacing.portPort": "22",
      "elk.layered.feedbackEdges": "true",
      "elk.layered.mergeEdges": "false",
      "elk.layered.cycleBreaking.strategy": "GREEDY_MODEL_ORDER",
      "elk.layered.considerModelOrder.strategy": "NODES_AND_EDGES",
      "elk.layered.crossingMinimization.forceNodeModelOrder": "true",
      "elk.layered.nodePlacement.strategy": "NETWORK_SIMPLEX",
      "elk.layered.crossingMinimization.strategy": "LAYER_SWEEP",
      "elk.layered.spacing.nodeNodeBetweenLayers": "130",
      "elk.layered.spacing.edgeNodeBetweenLayers": "42",
      "elk.layered.spacing.edgeEdgeBetweenLayers": "28",
    },
    children: nodes.map(nodeToElkNode),
    edges: edges
      .filter((edge) => nodeIds.has(edge.source) && nodeIds.has(edge.target))
      .map(edgeToElkEdge),
  };

  const elk = await getElk();
  const layout = await elk.layout(graph);
  const children = layout.children ?? [];
  const positions = new Map<string, { x: number; y: number }>();
  const originX = Math.min(...children.map((child) => child.x ?? 0), 0);
  const originY = Math.min(...children.map((child) => child.y ?? 0), 0);

  for (const child of children) {
    positions.set(child.id, {
      x: snapToFlowGrid((child.x ?? 0) - originX),
      y: snapToFlowGrid((child.y ?? 0) - originY),
    });
  }

  const layoutedNodes = nodes.map((node) => ({
    ...node,
    position: positions.get(node.id) ?? node.position,
  }));
  return enforceTimelinePositions(flow, layoutedNodes);
}

function enforceTimelinePositions(flow: FlowJSON, nodes: Node[]): Node[] {
  const ranks = computeExecutionRanks(flow);
  const roles = computeGraphRoles(flow);
  const nodePositions = new Map(
    nodes.map((node) => [node.id, node.position] as const)
  );
  const agentPositions = new Map<string, { x: number; y: number }>();

  for (const agent of flow.agents) {
    const rank = ranks.get(agent.name) ?? 0;
    const existing = nodePositions.get(agent.name);
    agentPositions.set(agent.name, {
      x: (rank + 1) * TIME_RANK_SPACING,
      y: existing?.y ?? agent.position[1] * GRID_SIZE,
    });
  }

  const outputIndexes = outputIndexesByNode(nodes);

  const positioned = nodes.map((node) => {
    const kind = (node.data as { kind?: string }).kind;
    if (kind === "loopFrame") {
      return node;
    }
    if (node.id === START_NODE_ID) {
      return {
        ...node,
        position: {
          x: 0,
          y: terminalY(flow, roles, agentPositions, "start", nodes),
        },
      };
    }
    if (node.id === END_NODE_ID) {
      return {
        ...node,
        position: {
          x: (Math.max(0, ...Array.from(ranks.values())) + 2) *
            TIME_RANK_SPACING,
          y: terminalY(flow, roles, agentPositions, "end", nodes),
        },
      };
    }

    if (agentPositions.has(node.id)) {
      return {
        ...node,
        position: agentPositions.get(node.id) ?? node.position,
      };
    }

    const data = node.data as {
      kind?: string;
      source?: string;
      target?: string;
    };
    if (data.kind === "goal" && data.target) {
      const target = agentPositions.get(data.target);
      if (!target) return node;
      return {
        ...node,
        position: {
          x: target.x + GOAL_CENTER_OFFSET_X,
          y: target.y - GOAL_OFFSET_Y,
        },
      };
    }
    if (data.kind === "input" && data.target) {
      const target = agentPositions.get(data.target);
      if (!target) return node;
      return {
        ...node,
        position: {
          x: target.x - INPUT_OFFSET_X,
          y: target.y + inputOffsetY(flow, node.id),
        },
      };
    }
    if (data.kind === "output" && data.source) {
      const source = agentPositions.get(data.source);
      if (!source) return node;
      const row = outputIndexes.get(node.id) ?? 0;
      return {
        ...node,
        position: {
          x: source.x + OUTPUT_OFFSET_X,
          y: source.y + AGENT_NODE_HEIGHT + 18 + row * (OUTPUT_NODE_HEIGHT + 14),
        },
      };
    }
    return node;
  });

  const loops = computeFeedbackLoops(ranks, computeConnectionModel(flow));
  const positionedById = new Map(
    positioned.map((node) => [node.id, node] as const)
  );

  return positioned.map((node) => {
    const data = node.data as { kind?: string };
    if (data.kind !== "loopFrame") return node;

    const loop = loops.find((candidate) => candidate.id === node.id);
    if (!loop) return node;
    const bounds = loopFrameBounds(flow, loop, positionedById);

    return {
      ...node,
      position: { x: bounds.x, y: bounds.y },
      zIndex: -20 + loop.depth,
      data: {
        ...(node.data as Record<string, unknown>),
        condition: loop.label,
        source: loop.source,
        target: loop.target,
        depth: loop.depth,
        loopColor: loop.color,
        width: bounds.width,
        height: bounds.height,
      },
    };
  });
}

function loopFrameBounds(
  flow: FlowJSON,
  loop: LoopSpec,
  nodesById: Map<string, Node>
): { x: number; y: number; width: number; height: number } {
  const agentNames = new Set(flow.agents.map((agent) => agent.name));
  const bodyAgents = new Set(
    Array.from(loop.body).filter((name) => agentNames.has(name))
  );

  const bodyNodes = Array.from(nodesById.values()).filter((node) =>
    nodeBelongsToLoopBody(node, bodyAgents)
  );
  if (bodyNodes.length === 0) {
    return {
      x: loop.startRank * TIME_RANK_SPACING,
      y: 0,
      width: LOOP_FRAME_MIN_WIDTH,
      height: LOOP_FRAME_MIN_HEIGHT,
    };
  }

  const rects = bodyNodes.map((node) => {
    const size = estimatedNodeSize(node);
    return {
      x: node.position.x,
      y: node.position.y,
      width: size.width,
      height: size.height,
    };
  });
  const minX = Math.min(...rects.map((rect) => rect.x));
  const minY = Math.min(...rects.map((rect) => rect.y));
  const maxX = Math.max(...rects.map((rect) => rect.x + rect.width));
  const maxY = Math.max(...rects.map((rect) => rect.y + rect.height));
  const nestedInset = loop.depth * LOOP_FRAME_NESTED_INSET;
  const baseX = minX - LOOP_FRAME_PADDING_X;
  const baseY = minY - LOOP_FRAME_PADDING_TOP;
  const baseWidth = maxX - minX + LOOP_FRAME_PADDING_X * 2;
  const baseHeight =
    maxY - minY + LOOP_FRAME_PADDING_TOP + LOOP_FRAME_PADDING_BOTTOM;
  const x = baseX + nestedInset;
  const y = baseY + nestedInset;
  const width = Math.max(
    LOOP_FRAME_MIN_WIDTH,
    baseWidth - nestedInset * 2
  );
  const height = Math.max(
    LOOP_FRAME_MIN_HEIGHT,
    baseHeight - nestedInset * 2
  );

  return {
    x: snapToFrameGrid(x),
    y: snapToFrameGrid(y),
    width: snapToFrameGrid(width),
    height: snapToFrameGrid(height),
  };
}

function nodeBelongsToLoopBody(node: Node, bodyAgents: Set<string>): boolean {
  if (bodyAgents.has(node.id)) return true;
  const data = node.data as {
    kind?: string;
    source?: string;
    target?: string;
  };
  if (data.kind === "input" && data.target) {
    return bodyAgents.has(data.target);
  }
  if (data.kind === "output" && data.source) {
    return bodyAgents.has(data.source);
  }
  if (data.kind === "goal" && data.target) {
    return bodyAgents.has(data.target);
  }
  return false;
}

function terminalY(
  flow: FlowJSON,
  roles: Map<string, AgentRole>,
  agentPositions: Map<string, { x: number; y: number }>,
  kind: "start" | "end",
  nodes: Node[]
): number {
  if (kind === "end") {
    return flowVisualBottom(flow, agentPositions, nodes) + EXIT_LANE_GAP;
  }

  const candidates = flow.agents
    .filter((agent) => {
      const role = roles.get(agent.name);
      return role?.isStart;
    })
    .map((agent) => agentPositions.get(agent.name)?.y)
    .filter((y): y is number => y !== undefined);
  if (candidates.length === 0) return 0;
  const avg = candidates.reduce((sum, y) => sum + y, 0) / candidates.length;
  return avg + (AGENT_NODE_HEIGHT - TERMINAL_NODE_HEIGHT) / 2;
}

function flowVisualBottom(
  flow: FlowJSON,
  agentPositions: Map<string, { x: number; y: number }>,
  nodes: Node[]
): number {
  const inputCounts = new Map<string, number>();
  const outputCounts = new Map<string, number>();

  for (const node of nodes) {
    const data = node.data as {
      kind?: string;
      source?: string;
      target?: string;
    };
    if (data.kind === "input" && data.target) {
      inputCounts.set(data.target, (inputCounts.get(data.target) ?? 0) + 1);
    }
    if (data.kind === "output" && data.source) {
      outputCounts.set(data.source, (outputCounts.get(data.source) ?? 0) + 1);
    }
  }

  return Math.max(
    0,
    ...flow.agents.map((agent) => {
      const position = agentPositions.get(agent.name);
      if (!position) return 0;
      const inputBottom = stackedSidecarBottom(
        inputCounts.get(agent.name) ?? 0,
        INPUT_NODE_HEIGHT,
        18
      );
      const outputBottom =
        AGENT_NODE_HEIGHT +
        stackedSidecarBottom(
          outputCounts.get(agent.name) ?? 0,
          OUTPUT_NODE_HEIGHT,
          18
        );
      return (
        position.y +
        Math.max(AGENT_NODE_HEIGHT, inputBottom, outputBottom)
      );
    })
  );
}

function stackedSidecarBottom(
  count: number,
  height: number,
  firstOffset: number
): number {
  if (count <= 0) return 0;
  return firstOffset + (count - 1) * (height + 14) + height;
}

function inputOffsetY(flow: FlowJSON, inputNodeIdValue: string): number {
  for (const agent of flow.agents) {
    const names = Object.keys(agent.inputs);
    const index = names.findIndex(
      (inputName) => inputNodeId(agent.name, inputName) === inputNodeIdValue
    );
    if (index >= 0) {
      return (
        AGENT_NODE_HEIGHT +
        SIDECAR_GAP_Y +
        index * (INPUT_NODE_HEIGHT + 12)
      );
    }
  }
  return AGENT_NODE_HEIGHT + SIDECAR_GAP_Y;
}

function outputIndexesByNode(nodes: Node[]): Map<string, number> {
  const counters = new Map<string, number>();
  const indexes = new Map<string, number>();
  for (const node of nodes) {
    const data = node.data as { kind?: string; source?: string };
    if (data.kind !== "output" || !data.source) continue;
    const next = counters.get(data.source) ?? 0;
    counters.set(data.source, next + 1);
    indexes.set(node.id, next);
  }
  return indexes;
}

export function nodesToFlow(
  flow: FlowJSON,
  nodes: Node[]
): FlowJSON {
  const agentMap = new Map<string, AgentJSON>();
  for (const agent of flow.agents) {
    agentMap.set(agent.name, { ...agent });
  }

  for (const node of nodes) {
    const agent = agentMap.get(node.id);
    if (agent) {
      agent.position = [
        Math.round(node.position.x / GRID_SIZE),
        Math.round(node.position.y / GRID_SIZE),
      ];
    }
  }

  return {
    ...flow,
    agents: flow.agents.map((a) => agentMap.get(a.name) || a),
  };
}

function nodeToElkNode(node: Node): ElkNode {
  const size = estimatedNodeSize(node);
  const data = node.data as { terminalKind?: string };
  const layerConstraint =
    data.terminalKind === "start"
      ? "FIRST"
      : data.terminalKind === "end"
      ? "LAST"
      : undefined;
  return {
    id: node.id,
    width: size.width,
    height: size.height,
    ports: portsForNode(node),
    layoutOptions: {
      "elk.portConstraints": "FIXED_ORDER",
      ...(layerConstraint
        ? { "elk.layered.layering.layerConstraint": layerConstraint }
        : {}),
    },
  };
}

function edgeToElkEdge(edge: Edge): ElkExtendedEdge {
  const label = typeof edge.label === "string" ? edge.label : "";
  const metadata =
    edge.data as { isFeedback?: boolean; isRoute?: boolean } | undefined;
  const priority = metadata?.isFeedback
    ? "1"
    : metadata?.isRoute
    ? "10"
    : "3";
  return {
    id: edge.id,
    sources: [portId(edge.source, edge.sourceHandle)],
    targets: [portId(edge.target, edge.targetHandle)],
    labels: label
      ? [{ text: label, width: estimateTextWidth(label, 11), height: 18 }]
      : undefined,
    layoutOptions: {
      "elk.layered.priority.direction": priority,
    },
  };
}

function portsForNode(node: Node): ElkPort[] {
  return [
    elkPort(node.id, "goal-in", "NORTH", 0),
    elkPort(node.id, "ctrl-in", "WEST", 0),
    elkPort(node.id, "data-in", "WEST", 1),
    elkPort(node.id, "ctrl-out", "EAST", 0),
    elkPort(node.id, "data-out", "EAST", 1),
    elkPort(node.id, "goal-out", "SOUTH", 0),
  ];
}

function elkPort(
  nodeId: string,
  handleId: string,
  side: "WEST" | "EAST" | "NORTH" | "SOUTH",
  index: number
): ElkPort {
  return {
    id: portId(nodeId, handleId),
    width: PORT_SIZE,
    height: PORT_SIZE,
    layoutOptions: {
      "elk.port.side": side,
      "elk.port.index": String(index),
    },
  };
}

function portId(nodeId: string, handleId?: string | null): string {
  return handleId ? `${nodeId}:${handleId}` : nodeId;
}

function estimatedNodeSize(node: Node): { width: number; height: number } {
  const kind = (node.data as { kind?: string }).kind;
  if (kind === "input") {
    const name = String((node.data as { name?: unknown }).name ?? node.id);
    return {
      width: Math.max(INPUT_NODE_WIDTH, estimateTextWidth(name, 11) + 120),
      height: INPUT_NODE_HEIGHT,
    };
  }
  if (kind === "output") {
    const name = String((node.data as { name?: unknown }).name ?? node.id);
    return {
      width: Math.max(OUTPUT_NODE_WIDTH, estimateTextWidth(name, 11) + 112),
      height: OUTPUT_NODE_HEIGHT,
    };
  }
  if (kind === "goal") {
    return {
      width: GOAL_NODE_WIDTH,
      height: GOAL_NODE_HEIGHT,
    };
  }
  if (kind === "terminal") {
    return {
      width: TERMINAL_NODE_WIDTH,
      height: TERMINAL_NODE_HEIGHT,
    };
  }
  if (kind === "loopFrame") {
    const data = node.data as { width?: number; height?: number };
    return {
      width: data.width ?? LOOP_FRAME_MIN_WIDTH,
      height: data.height ?? LOOP_FRAME_MIN_HEIGHT,
    };
  }

  const agent = (node.data as { agent?: AgentJSON }).agent;
  const name = agent?.name ?? node.id;
  const language =
    agent?.node_type === "function" ? agent.language ?? "" : "prompt";
  return {
    width: Math.max(
      AGENT_NODE_WIDTH,
      estimateTextWidth(name, 13) + estimateTextWidth(language, 11) + 96
    ),
    height: AGENT_NODE_HEIGHT,
  };
}

function estimateTextWidth(text: string, fontSize: number): number {
  return Math.min(360, Math.ceil(text.length * fontSize * 0.62));
}

function snapToFlowGrid(value: number): number {
  return Math.round(value / GRID_SIZE) * GRID_SIZE;
}

function snapToFrameGrid(value: number): number {
  return Math.round(value / 20) * 20;
}

function getElk(): Promise<ELK> {
  if (!elkPromise) {
    elkPromise = import("elkjs/lib/elk.bundled.js").then(
      ({ default: ELKConstructor }) => new ELKConstructor()
    );
  }
  return elkPromise;
}
