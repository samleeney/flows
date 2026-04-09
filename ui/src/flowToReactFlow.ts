import type { Node, Edge } from "@xyflow/react";
import dagre from "dagre";
import type { FlowJSON, AgentJSON } from "./types";

const GRID_SIZE = 200;
const NODE_WIDTH = 180;
const NODE_HEIGHT = 80;

export function flowToNodes(flow: FlowJSON): Node[] {
  return flow.agents.map((agent) => ({
    id: agent.name,
    type: agent.node_type === "function" ? "functionNode" : "agentNode",
    position: {
      x: agent.position[0] * GRID_SIZE,
      y: agent.position[1] * GRID_SIZE,
    },
    data: { agent },
  }));
}

export function flowToEdges(flow: FlowJSON): Edge[] {
  const edges: Edge[] = [];
  const seen = new Set<string>();

  for (const agent of flow.agents) {
    // Edges from start conditions
    for (const cond of agent.start) {
      if (cond.when) {
        for (const dep of cond.when) {
          const key = `${dep}->${agent.name}`;
          if (seen.has(key)) continue;
          seen.add(key);

          let label = "";
          if (cond.contains) label += `"${cond.contains}"`;
          if (cond.max_runs) {
            label += label ? `, max ${cond.max_runs}` : `max ${cond.max_runs}`;
          }

          edges.push({
            id: key,
            source: dep,
            target: agent.name,
            label: label || undefined,
            animated: !!cond.contains,
            style: { strokeWidth: 2 },
          });
        }
      }
    }

    // Data flow edges from inputs
    for (const [, input] of Object.entries(agent.inputs)) {
      if (input.from === "external" || !input.from) continue;
      const key = `${input.from}->${agent.name}`;
      if (seen.has(key)) continue;
      seen.add(key);
      edges.push({
        id: `data-${key}`,
        source: input.from,
        target: agent.name,
        style: { strokeWidth: 1, strokeDasharray: "5 5" },
      });
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
