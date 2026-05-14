import { MarkerType, type Node, type Edge } from "@xyflow/react";
import type { ELK, ElkExtendedEdge, ElkNode, ElkPort } from "elkjs";
import type { FlowJSON, AgentJSON, AgentLiveState } from "./types";

const GRID_SIZE = 100;
const AGENT_NODE_WIDTH = 280;
const AGENT_NODE_HEIGHT = 132;
const INPUT_NODE_WIDTH = 180;
const INPUT_NODE_HEIGHT = 42;
const OUTPUT_NODE_WIDTH = 170;
const OUTPUT_NODE_HEIGHT = 46;
const PORT_SIZE = 8;

const CONTROL_COLOR = "#334155";
const DATA_COLOR = "#0d9488";
const LOOP_COLOR = "#e11d48";

let elkPromise: Promise<ELK> | null = null;

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

  const agentNodes: Node[] = flow.agents.map((agent) => ({
    id: agent.name,
    type: agent.node_type === "function" ? "functionNode" : "agentNode",
    position: {
      x: agent.position[0] * GRID_SIZE,
      y: agent.position[1] * GRID_SIZE,
    },
    data: {
      agent,
      label: labels.get(agent.name) ?? "",
      liveState: agentLive?.[agent.name],
      role: roles.get(agent.name),
      inputs: summarizeInputs(agent),
    },
  }));

  const inputNodes: Node[] = flow.agents.flatMap((agent) =>
    Object.entries(agent.inputs).map(([inputName, input], i) => ({
      id: inputNodeId(agent.name, inputName),
      type: "inputNode",
      position: {
        x: (agent.position[0] - 1) * GRID_SIZE,
        y: (agent.position[1] + i) * GRID_SIZE,
      },
      data: {
        kind: "input",
        name: inputName,
        target: agent.name,
        source: sourceLabel(input.from),
        sourceKind: sourceKind(input.from),
        fallback: input.fallback ? sourceLabel(input.fallback) : undefined,
        fallbackKind: input.fallback ? sourceKind(input.fallback) : undefined,
      },
    }))
  );

  const outputNodes: Node[] = connections.dataConnections.map((conn) => ({
    id: outputNodeId(conn.source, conn.target, conn.inputName, conn.isFallback),
    type: "outputNode",
    position: { x: 0, y: 0 },
    data: {
      kind: "output",
      name: conn.label,
      source: conn.source,
      target: conn.target,
      inputName: conn.inputName,
      isFallback: conn.isFallback,
      condition: conditionSummary(
        connections.triggerSpecs.get(edgeKey(conn.source, conn.target))
      ),
    },
  }));

  const triggerOutputNodes: Node[] = connections.triggerOnly.map((trigger) => ({
    id: triggerOutputNodeId(trigger.source, trigger.target),
    type: "outputNode",
    position: { x: 0, y: 0 },
    data: {
      kind: "output",
      name: "trigger",
      source: trigger.source,
      target: trigger.target,
      condition: conditionSummary(trigger),
    },
  }));

  return [...agentNodes, ...inputNodes, ...outputNodes, ...triggerOutputNodes];
}

// decorateNodes returns a copy of nodes with each node's liveState updated
// from the given agent map. Used to refresh nodes in place when live events
// arrive, without rebuilding from scratch.
export function decorateNodes(
  nodes: Node[],
  agentLive?: Record<string, AgentLiveState>
): Node[] {
  return nodes.map((n) => ({
    ...n,
    data: {
      ...(n.data as Record<string, unknown>),
      liveState: agentLive?.[n.id],
    },
  }));
}

export function flowToEdges(flow: FlowJSON): Edge[] {
  const edges: Edge[] = [];
  const ranks = computeExecutionRanks(flow);
  const connections = computeConnectionModel(flow);

  for (const agent of flow.agents) {
    for (const inputName of Object.keys(agent.inputs)) {
      edges.push(inputToBlockEdge(agent.name, inputName));
    }
  }

  for (const conn of connections.dataConnections) {
    const trigger = connections.triggerSpecs.get(edgeKey(conn.source, conn.target));
    const isFeedback = isFeedbackEdge(ranks, conn.source, conn.target);
    edges.push(sourceToOutputEdge(conn, isFeedback));
    edges.push(outputToInputEdge(conn, trigger, isFeedback));
  }

  for (const spec of connections.triggerOnly) {
    const isFeedback = isFeedbackEdge(ranks, spec.source, spec.target);
    edges.push(sourceToTriggerOutputEdge(spec, isFeedback));
    edges.push(triggerOutputToBlockEdge(spec, isFeedback));
  }

  return edges;
}

interface InputSummary {
  name: string;
  source: string;
  sourceKind: "agent" | "external";
  fallback?: string;
  fallbackKind?: "agent" | "external";
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
  labels: string[];
  isConditional: boolean;
}

interface ConnectionModel {
  dataConnections: DataConnectionSpec[];
  triggerSpecs: Map<string, TriggerSpec>;
  triggerOnly: TriggerSpec[];
}

function summarizeInputs(agent: AgentJSON): InputSummary[] {
  return Object.entries(agent.inputs).map(([name, input]) => ({
    name,
    source: sourceLabel(input.from),
    sourceKind: sourceKind(input.from),
    fallback: input.fallback ? sourceLabel(input.fallback) : undefined,
    fallbackKind: input.fallback ? sourceKind(input.fallback) : undefined,
  }));
}

function sourceLabel(source: string): string {
  return source === "external" ? "external" : source;
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
  const label = conditionLabel(cond);
  if (existing) {
    if (label) existing.labels.push(label);
    existing.isConditional =
      existing.isConditional || Boolean(cond.contains || cond.max_runs);
  } else {
    triggerEdges.set(key, {
      source,
      target,
      labels: label ? [label] : [],
      isConditional: Boolean(cond.contains || cond.max_runs),
    });
  }
}

function conditionLabel(cond: AgentJSON["start"][number]): string {
  const parts: string[] = [];
  if (cond.contains) parts.push(`"${cond.contains}"`);
  if (cond.max_runs) parts.push(`max ${cond.max_runs}`);
  return parts.join(", ");
}

function edgeKey(source: string, target: string): string {
  return `${source}->${target}`;
}

function conditionSummary(trigger: TriggerSpec | undefined): string | undefined {
  if (!trigger) return undefined;
  return formatTriggerLabel(trigger, false) || undefined;
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
    data: { isLocalInput: true },
  } as Edge;
}

function sourceToOutputEdge(
  spec: DataConnectionSpec,
  isFeedback: boolean
): Edge {
  const color = isFeedback ? LOOP_COLOR : DATA_COLOR;
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
    animated: isFeedback,
    interactionWidth: 16,
    pathOptions: { offset: isFeedback ? 48 : 28, borderRadius: 10 },
    style: {
      strokeWidth: isFeedback ? 2.1 : 1.6,
      stroke: color,
      strokeDasharray: isFeedback ? undefined : "5 4",
    },
    markerEnd: { type: MarkerType.Arrow, color },
    data: { isFeedback, isOutput: true },
  } as Edge;
}

function outputToInputEdge(
  spec: DataConnectionSpec,
  trigger: TriggerSpec | undefined,
  isFeedback: boolean
): Edge {
  const triggerLabel = trigger ? formatTriggerLabel(trigger, isFeedback) : "";
  const color = isFeedback ? LOOP_COLOR : DATA_COLOR;
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
    label: triggerLabel || undefined,
    animated: isFeedback,
    interactionWidth: 18,
    pathOptions: { offset: isFeedback ? 58 : 28, borderRadius: 10 },
    labelStyle: { fill: color, fontSize: 11, fontWeight: 700 },
    labelBgStyle: {
      fill: isFeedback ? "#fff1f2" : "#f0fdfa",
      fillOpacity: 0.94,
    },
    style: {
      strokeWidth: isFeedback ? 2.3 : 1.7,
      stroke: color,
      strokeDasharray: isFeedback ? undefined : "5 4",
    },
    markerEnd: {
      type: isFeedback ? MarkerType.ArrowClosed : MarkerType.Arrow,
      color,
    },
    data: { isFeedback, isOutput: true },
  } as Edge;
}

function sourceToTriggerOutputEdge(
  spec: TriggerSpec,
  isFeedback: boolean
): Edge {
  const color = isFeedback ? LOOP_COLOR : CONTROL_COLOR;
  return {
    id: `${spec.source}->${triggerOutputNodeId(spec.source, spec.target)}`,
    type: "smoothstep",
    source: spec.source,
    target: triggerOutputNodeId(spec.source, spec.target),
    sourceHandle: "ctrl-out",
    targetHandle: "data-in",
    animated: isFeedback || spec.isConditional,
    interactionWidth: 16,
    pathOptions: { offset: isFeedback ? 58 : 34, borderRadius: 10 },
    style: {
      strokeWidth: isFeedback ? 2.3 : 1.8,
      stroke: color,
      strokeDasharray: isFeedback ? undefined : "8 5",
    },
    markerEnd: { type: MarkerType.ArrowClosed, color },
    data: { isFeedback, isTrigger: true },
  } as Edge;
}

function triggerOutputToBlockEdge(
  spec: TriggerSpec,
  isFeedback: boolean
): Edge {
  const color = isFeedback ? LOOP_COLOR : CONTROL_COLOR;
  return {
    id: `${triggerOutputNodeId(spec.source, spec.target)}->${spec.target}`,
    type: "smoothstep",
    source: triggerOutputNodeId(spec.source, spec.target),
    target: spec.target,
    sourceHandle: "ctrl-out",
    targetHandle: "ctrl-in",
    label: formatTriggerLabel(spec, isFeedback) || undefined,
    animated: isFeedback || spec.isConditional,
    interactionWidth: 18,
    pathOptions: { offset: isFeedback ? 58 : 34, borderRadius: 10 },
    labelStyle: { fill: color, fontSize: 11, fontWeight: 700 },
    labelBgStyle: {
      fill: isFeedback ? "#fff1f2" : "#f8fafc",
      fillOpacity: 0.94,
    },
    style: {
      strokeWidth: isFeedback ? 2.4 : 2,
      stroke: color,
      strokeDasharray: isFeedback ? undefined : "8 5",
    },
    markerEnd: { type: MarkerType.ArrowClosed, color },
    data: { isFeedback, isTrigger: true },
  } as Edge;
}

function formatTriggerLabel(spec: TriggerSpec, isFeedback: boolean): string {
  const label = spec.labels.join(" / ");
  if (!label) return isFeedback ? "LOOP" : "";
  if (isFeedback) {
    return label.startsWith('"') ? `LOOP if ${label}` : `LOOP ${label}`;
  }
  return spec.isConditional ? `if ${label}` : label;
}

export async function autoLayout(nodes: Node[], edges: Edge[]): Promise<Node[]> {
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

  return nodes.map((node) => ({
    ...node,
    position: positions.get(node.id) ?? node.position,
  }));
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
  return {
    id: node.id,
    width: size.width,
    height: size.height,
    ports: portsForNode(node),
    layoutOptions: {
      "elk.portConstraints": "FIXED_ORDER",
    },
  };
}

function edgeToElkEdge(edge: Edge): ElkExtendedEdge {
  const label = typeof edge.label === "string" ? edge.label : "";
  const metadata = edge.data as { isFeedback?: boolean } | undefined;
  const priority = metadata?.isFeedback
    ? "1"
    : edge.id.startsWith("ctrl:")
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
    elkPort(node.id, "ctrl-in", "WEST", 0),
    elkPort(node.id, "data-in", "WEST", 1),
    elkPort(node.id, "ctrl-out", "EAST", 0),
    elkPort(node.id, "data-out", "EAST", 1),
  ];
}

function elkPort(
  nodeId: string,
  handleId: string,
  side: "WEST" | "EAST",
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

function getElk(): Promise<ELK> {
  if (!elkPromise) {
    elkPromise = import("elkjs/lib/elk.bundled.js").then(
      ({ default: ELKConstructor }) => new ELKConstructor()
    );
  }
  return elkPromise;
}
