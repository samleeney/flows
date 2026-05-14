import { MarkerType, type Node, type Edge } from "@xyflow/react";
import dagre from "dagre";
import type { FlowJSON, AgentJSON, AgentLiveState } from "./types";

const GRID_SIZE = 200;
const NODE_WIDTH = 180;
const NODE_HEIGHT = 80;

const CONTROL_COLOR = "#334155";
const DATA_COLOR = "#0d9488";

export const externalNodeId = (name: string) => `external:${name}`;

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

  // Compute which external inputs are actually consumed (so we don't
  // render orphan pseudo-nodes for declared-but-unused inputs).
  const usedExternals = new Set<string>();
  for (const agent of flow.agents) {
    for (const [inputName, input] of Object.entries(agent.inputs)) {
      if (input.from === "external") usedExternals.add(inputName);
    }
  }

  const externalNodes: Node[] = (flow.external_inputs ?? [])
    .filter((name) => usedExternals.has(name))
    .map((name, i) => ({
      id: externalNodeId(name),
      type: "externalInputNode",
      position: { x: -2 * GRID_SIZE, y: i * GRID_SIZE },
      data: { name },
    }));

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
    },
  }));

  return [...externalNodes, ...agentNodes];
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
  const controlSeen = new Set<string>();
  // Group data inputs from the same source so we emit one edge with a
  // comma-joined label instead of stacked duplicates.
  const dataLabels = new Map<string, string[]>();

  for (const agent of flow.agents) {
    // Control edges from start.when conditions.
    for (const cond of agent.start) {
      if (!cond.when) continue;
      for (const dep of cond.when) {
        const key = `ctrl:${dep}->${agent.name}`;
        if (controlSeen.has(key)) continue;
        controlSeen.add(key);

        let label = "";
        if (cond.contains) label += `"${cond.contains}"`;
        if (cond.max_runs) {
          label += label ? `, max ${cond.max_runs}` : `max ${cond.max_runs}`;
        }

        edges.push({
          id: key,
          source: dep,
          target: agent.name,
          sourceHandle: "ctrl-out",
          targetHandle: "ctrl-in",
          label: label || undefined,
          animated: !!cond.contains,
          style: { strokeWidth: 2, stroke: CONTROL_COLOR },
          markerEnd: { type: MarkerType.ArrowClosed, color: CONTROL_COLOR },
        });
      }
    }

    // Data edges from declared inputs. External inputs source from a
    // pseudo-node; agent-to-agent inputs source from the producer.
    for (const [inputName, input] of Object.entries(agent.inputs)) {
      if (!input.from) continue;
      const sourceId =
        input.from === "external" ? externalNodeId(inputName) : input.from;
      const key = `data:${sourceId}->${agent.name}`;
      const existing = dataLabels.get(key);
      if (existing) {
        existing.push(inputName);
      } else {
        dataLabels.set(key, [inputName]);
      }
    }
  }

  for (const agent of flow.agents) {
    for (const [inputName, input] of Object.entries(agent.inputs)) {
      if (!input.from) continue;
      const sourceId =
        input.from === "external" ? externalNodeId(inputName) : input.from;
      const key = `data:${sourceId}->${agent.name}`;
      const labels = dataLabels.get(key);
      if (!labels) continue;
      // Emit once per (source, target) pair.
      dataLabels.delete(key);
      edges.push({
        id: key,
        source: sourceId,
        target: agent.name,
        sourceHandle: "data-out",
        targetHandle: "data-in",
        label: labels.join(", "),
        labelStyle: { fill: DATA_COLOR, fontSize: 11, fontWeight: 500 },
        labelBgStyle: { fill: "#f0fdfa", fillOpacity: 0.9 },
        style: {
          strokeWidth: 1.5,
          stroke: DATA_COLOR,
          strokeDasharray: "5 4",
        },
        markerEnd: { type: MarkerType.Arrow, color: DATA_COLOR },
      });
      // Touch inputName to avoid lint complaint when only used in the map key.
      void inputName;
    }
  }

  return edges;
}

export function autoLayout(nodes: Node[], edges: Edge[]): Node[] {
  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  g.setGraph({ rankdir: "LR", nodesep: 50, ranksep: 100 });

  for (const node of nodes) {
    g.setNode(node.id, { width: NODE_WIDTH, height: NODE_HEIGHT });
  }
  for (const edge of edges) {
    g.setEdge(edge.source, edge.target);
  }

  dagre.layout(g);

  return nodes.map((node) => {
    const pos = g.node(node.id);
    return {
      ...node,
      position: { x: pos.x - NODE_WIDTH / 2, y: pos.y - NODE_HEIGHT / 2 },
    };
  });
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
