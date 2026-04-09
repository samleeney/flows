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

import type { FlowJSON } from "./types";
import { useFlowWebSocket } from "./useFlowWebSocket";
import {
  flowToNodes,
  flowToEdges,
  autoLayout,
  nodesToFlow,
} from "./flowToReactFlow";
import { AgentNode, FunctionNode } from "./AgentNode";

const nodeTypes = {
  agentNode: AgentNode,
  functionNode: FunctionNode,
};

export default function App() {
  const [flow, setFlow] = useState<FlowJSON | null>(null);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [edges, setEdges] = useState<Edge[]>([]);

  const onFlowUpdate = useCallback((newFlow: FlowJSON) => {
    setFlow(newFlow);
    let newNodes = flowToNodes(newFlow);
    const newEdges = flowToEdges(newFlow);

    // Auto-layout if all positions are [0,0]
    const allZero = newFlow.agents.every(
      (a) => a.position[0] === 0 && a.position[1] === 0
    );
    if (allZero && newFlow.agents.length > 1) {
      newNodes = autoLayout(newNodes, newEdges);
    }

    setNodes(newNodes);
    setEdges(newEdges);
  }, []);

  const { sendUpdate } = useFlowWebSocket(onFlowUpdate);

  const onNodesChange = useCallback(
    (changes: NodeChange[]) => {
      setNodes((nds) => {
        const updated = applyNodeChanges(changes, nds);

        // If positions changed, sync back to server
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

  const defaultEdgeOptions = useMemo(
    () => ({
      style: { strokeWidth: 2, stroke: "#64748b" },
    }),
    []
  );

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
      </div>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        nodeTypes={nodeTypes}
        defaultEdgeOptions={defaultEdgeOptions}
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
