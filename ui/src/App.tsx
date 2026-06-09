import { useState, useCallback, useMemo, useRef, useEffect } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  type Node,
  type Edge,
  type ReactFlowInstance,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import type {
  FlowJSON,
  LiveUIState,
  RunSnapshot,
  EventEnvelope,
} from "./types";
import { useFlowWebSocket } from "./useFlowWebSocket";
import {
  flowToNodes,
  flowToEdges,
  autoLayout,
  nodesToFlow,
  decorateNodes,
} from "./flowToReactFlow";
import { AgentNode, FunctionNode } from "./AgentNode";
import {
  InputNode,
  LoopFrameNode,
  OutputNode,
  TerminalNode,
} from "./SidecarNode";
import { ExitRouteEdge } from "./RouteEdges";
import {
  applyEvent,
  applySnapshot,
  emptyLiveState,
} from "./runState";

const nodeTypes = {
  agentNode: AgentNode,
  functionNode: FunctionNode,
  inputNode: InputNode,
  outputNode: OutputNode,
  terminalNode: TerminalNode,
  loopFrameNode: LoopFrameNode,
};

const edgeTypes = {
  exitRoute: ExitRouteEdge,
};

const FIT_VIEW_OPTIONS = {
  padding: 0.12,
  minZoom: 0.05,
  maxZoom: 1.2,
  duration: 180,
};

export default function App() {
  const [flow, setFlow] = useState<FlowJSON | null>(null);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [edges, setEdges] = useState<Edge[]>([]);
  const [liveState, setLiveState] = useState<LiveUIState>(emptyLiveState);
  const [selectedNode, setSelectedNode] = useState<Node | null>(null);
  const layoutSeq = useRef(0);
  const sendUpdateRef = useRef<(flow: FlowJSON) => void>(() => {});
  const reactFlowRef = useRef<ReactFlowInstance<Node, Edge> | null>(null);

  const onFlowUpdate = useCallback((newFlow: FlowJSON) => {
    const seq = ++layoutSeq.current;
    const newNodes = flowToNodes(newFlow);
    const newEdges = flowToEdges(newFlow);

    void autoLayout(newFlow, newNodes, newEdges)
      .then((layoutedNodes) => {
        if (layoutSeq.current !== seq) return;
        const layoutedFlow = nodesToFlow(newFlow, layoutedNodes);
        setFlow(layoutedFlow);
        setNodes(layoutedNodes);
        setEdges(newEdges);
        if (!sameAgentPositions(newFlow, layoutedFlow)) {
          sendUpdateRef.current(layoutedFlow);
        }
      })
      .catch((err) => {
        console.error("ELK layout failed", err);
      });
  }, []);

  const onRunSnapshot = useCallback((snap: RunSnapshot) => {
    setLiveState((cur) => applySnapshot(cur, snap));
  }, []);

  const onRunEvent = useCallback((env: EventEnvelope) => {
    setLiveState((cur) => applyEvent(cur, env));
  }, []);

  const { sendUpdate } = useFlowWebSocket({
    onFlowUpdate,
    onRunSnapshot,
    onRunEvent,
  });

  useEffect(() => {
    sendUpdateRef.current = sendUpdate;
  }, [sendUpdate]);

  useEffect(() => {
    if (nodes.length === 0 || !reactFlowRef.current) return;
    return scheduleFitView(reactFlowRef.current);
  }, [nodes, edges]);

  const decoratedNodes = useMemo(() => {
    const runId = liveState.selectedRunId;
    const run = runId ? liveState.runsById[runId] : undefined;
    return decorateNodes(nodes, run?.agents);
  }, [nodes, liveState]);

  if (!flow) {
    return (
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          height: "100vh",
          fontFamily: "system-ui",
          color: "#64748b",
        }}
      >
        Connecting...
      </div>
    );
  }

  const selectedRun = liveState.selectedRunId
    ? liveState.runsById[liveState.selectedRunId]
    : undefined;
  const runStatusLabel = selectedRun
    ? selectedRun.finished_at
      ? selectedRun.ok
        ? "run complete"
        : selectedRun.disconnected
        ? "run disconnected"
        : "run failed"
      : "running…"
    : null;

  return (
    <div style={{ width: "100vw", height: "100vh" }}>
      <div
        style={{
          position: "absolute",
          top: 12,
          left: 12,
          zIndex: 10,
          background: "white",
          padding: "8px 16px",
          borderRadius: 6,
          boxShadow: "0 1px 3px rgba(0,0,0,0.1)",
          fontFamily: "system-ui",
        }}
      >
        <strong>{flow.name}</strong>
        {flow.description && (
          <span style={{ color: "#94a3b8", marginLeft: 8 }}>
            {flow.description}
          </span>
        )}
        {runStatusLabel && (
          <span
            style={{
              marginLeft: 12,
              fontSize: 11,
              padding: "2px 8px",
              borderRadius: 999,
              background: "#f1f5f9",
              color: "#475569",
            }}
          >
            {runStatusLabel}
          </span>
        )}
      </div>
      {selectedNode && (
        <DetailsPanel
          node={selectedNode}
          onClose={() => setSelectedNode(null)}
        />
      )}
      <ReactFlow
        nodes={decoratedNodes}
        edges={edges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        onNodeClick={(_, node) => setSelectedNode(node)}
        onPaneClick={() => setSelectedNode(null)}
        onInit={(instance) => {
          reactFlowRef.current = instance;
          scheduleFitView(instance);
        }}
        nodesDraggable={false}
        nodesConnectable={false}
        panOnScroll
        panOnScrollSpeed={0.9}
        snapToGrid
        snapGrid={[20, 20]}
        fitView
        fitViewOptions={FIT_VIEW_OPTIONS}
      >
        <Background gap={20} size={1} />
        <Controls />
      </ReactFlow>
    </div>
  );
}

function DetailsPanel({
  node,
  onClose,
}: {
  node: Node;
  onClose: () => void;
}) {
  const data = node.data as Record<string, unknown>;
  const agent = data.agent as FlowJSON["agents"][number] | undefined;
  const kind = data.kind as string | undefined;

  return (
    <div
      style={{
        position: "absolute",
        top: 56,
        right: 16,
        zIndex: 20,
        width: 340,
        maxHeight: "calc(100vh - 80px)",
        overflow: "auto",
        border: "1px solid #cbd5e1",
        borderRadius: 8,
        background: "white",
        boxShadow: "0 12px 32px rgba(15, 23, 42, 0.16)",
        fontFamily: "system-ui",
        color: "#334155",
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          padding: "10px 12px",
          borderBottom: "1px solid #e2e8f0",
        }}
      >
        <strong style={{ fontSize: 13 }}>
          {agent?.name ?? String(data.name ?? data.label ?? node.id)}
        </strong>
        <button
          type="button"
          onClick={onClose}
          style={{
            border: "none",
            background: "transparent",
            color: "#64748b",
            cursor: "pointer",
            fontSize: 16,
            lineHeight: "16px",
          }}
          aria-label="Close details"
        >
          ×
        </button>
      </div>
      <div style={{ padding: 12, display: "grid", gap: 10, fontSize: 12 }}>
        {agent ? <AgentDetails agent={agent} data={data} /> : null}
        {kind === "input" ? <InputDetails data={data} /> : null}
        {kind === "output" ? <OutputDetails data={data} /> : null}
        {kind === "terminal" ? <TerminalDetails data={data} /> : null}
      </div>
    </div>
  );
}

function DetailRow({ label, value }: { label: string; value?: unknown }) {
  if (value === undefined || value === "") return null;
  return (
    <div>
      <div style={{ fontSize: 10, color: "#94a3b8", fontWeight: 700 }}>
        {label}
      </div>
      <div
        style={{
          marginTop: 2,
          overflowWrap: "anywhere",
          fontFamily:
            typeof value === "string" && value.includes("\n")
              ? "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace"
              : undefined,
          whiteSpace:
            typeof value === "string" && value.includes("\n") ? "pre-wrap" : undefined,
        }}
      >
        {String(value)}
      </div>
    </div>
  );
}

function AgentDetails({
  agent,
  data,
}: {
  agent: FlowJSON["agents"][number];
  data: Record<string, unknown>;
}) {
  const inputs = data.inputs as
    | Array<{ name: string; source: string; fallback?: string }>
    | undefined;
  return (
    <>
      <DetailRow
        label="type"
        value={
          agent.node_type === "function"
            ? `${agent.language ?? "code"} code block`
            : "agent prompt"
        }
      />
      <DetailRow
        label="inputs"
        value={inputs
          ?.map((input) =>
            input.fallback
              ? `${input.name}: ${input.source} else ${input.fallback}`
              : `${input.name}: ${input.source}`
          )
          .join("\n")}
      />
      <DetailRow
        label="starts"
        value={agent.start
          .map((start) => formatStartCondition(start))
          .join("\n")}
      />
      <DetailRow label="content" value={agent.content.slice(0, 1200)} />
    </>
  );
}

function formatStartCondition(start: FlowJSON["agents"][number]["start"][number]) {
  if (start.always) {
    return start.always.max_runs > 1
      ? `start immediately, up to ${start.always.max_runs} runs`
      : "start immediately";
  }

  const parts = [`after ${(start.when ?? []).join(", ")}`];
  if (start.contains) {
    parts.push(`if output has "${start.contains}"`);
  }
  if (start.max_runs) {
    parts.push(`up to ${start.max_runs} repeats`);
  }
  return parts.join(", ");
}

function InputDetails({ data }: { data: Record<string, unknown> }) {
  return (
    <>
      <DetailRow label="kind" value="input" />
      <DetailRow label="receiving block" value={data.target} />
      <DetailRow label="source" value={data.source} />
      <DetailRow label="fallback" value={data.fallback} />
    </>
  );
}

function OutputDetails({ data }: { data: Record<string, unknown> }) {
  return (
    <>
      <DetailRow label="kind" value="output" />
      <DetailRow label="generated by" value={data.source} />
      <DetailRow label="goes to" value={data.target} />
      <DetailRow label="target input" value={data.inputName} />
      <DetailRow label="condition" value={data.condition} />
    </>
  );
}

function TerminalDetails({ data }: { data: Record<string, unknown> }) {
  return (
    <>
      <DetailRow label="kind" value={data.label} />
      <DetailRow label="description" value={data.description} />
    </>
  );
}

function scheduleFitView(instance: ReactFlowInstance<Node, Edge>): () => void {
  let innerFrame = 0;
  const outerFrame = window.requestAnimationFrame(() => {
    innerFrame = window.requestAnimationFrame(() => {
      void instance.fitView(FIT_VIEW_OPTIONS);
    });
  });
  return () => {
    window.cancelAnimationFrame(outerFrame);
    if (innerFrame) window.cancelAnimationFrame(innerFrame);
  };
}

function sameAgentPositions(a: FlowJSON, b: FlowJSON): boolean {
  if (a.agents.length !== b.agents.length) return false;
  const oldPositions = new Map(
    a.agents.map((agent) => [agent.name, agent.position] as const)
  );
  return b.agents.every((agent) => {
    const old = oldPositions.get(agent.name);
    return old?.[0] === agent.position[0] && old?.[1] === agent.position[1];
  });
}
