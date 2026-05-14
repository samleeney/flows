import { useState, useCallback, useMemo } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  type NodeChange,
  applyNodeChanges,
  type Node,
  type Edge,
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
import { ExternalInputNode } from "./ExternalInputNode";
import {
  applyEvent,
  applySnapshot,
  emptyLiveState,
} from "./runState";

const nodeTypes = {
  agentNode: AgentNode,
  functionNode: FunctionNode,
  externalInputNode: ExternalInputNode,
};

export default function App() {
  const [flow, setFlow] = useState<FlowJSON | null>(null);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [edges, setEdges] = useState<Edge[]>([]);
  const [liveState, setLiveState] = useState<LiveUIState>(emptyLiveState);

  const onFlowUpdate = useCallback((newFlow: FlowJSON) => {
    setFlow(newFlow);
    let newNodes = flowToNodes(newFlow);
    const newEdges = flowToEdges(newFlow);

    const allZero = newFlow.agents.every(
      (a) => a.position[0] === 0 && a.position[1] === 0
    );
    if (allZero && newFlow.agents.length > 1) {
      newNodes = autoLayout(newNodes, newEdges);
    }

    setNodes(newNodes);
    setEdges(newEdges);
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

  const onNodesChange = useCallback(
    (changes: NodeChange[]) => {
      setNodes((nds) => {
        const updated = applyNodeChanges(changes, nds);

        const hasPosChange = changes.some(
          (c) => c.type === "position" && "position" in c && c.position
        );
        if (hasPosChange && flow) {
          const updatedFlow = nodesToFlow(flow, updated);
          setFlow(updatedFlow);
          sendUpdate(updatedFlow);
        }

        return updated;
      });
    },
    [flow, sendUpdate]
  );

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
      <ReactFlow
        nodes={decoratedNodes}
        edges={edges}
        onNodesChange={onNodesChange}
        nodeTypes={nodeTypes}
        snapToGrid
        snapGrid={[20, 20]}
        fitView
      >
        <Background gap={20} size={1} />
        <Controls />
      </ReactFlow>
    </div>
  );
}
