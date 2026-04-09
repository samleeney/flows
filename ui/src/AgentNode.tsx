import { Handle, Position } from "@xyflow/react";
import type { AgentJSON } from "./types";

interface AgentNodeProps {
  data: { agent: AgentJSON };
}

export function AgentNode({ data }: AgentNodeProps) {
  const { agent } = data;
  const isFunction = agent.node_type === "function";

  return (
    <div
      style={{
        padding: "12px 16px",
        border: `2px solid ${isFunction ? "#6366f1" : "#3b82f6"}`,
        borderRadius: isFunction ? "12px" : "6px",
        background: isFunction ? "#eef2ff" : "#eff6ff",
        minWidth: 160,
        fontFamily: "system-ui, sans-serif",
      }}
    >
      <Handle type="target" position={Position.Left} />
      <div style={{ fontWeight: 600, marginBottom: 4 }}>{agent.name}</div>
      <div style={{ fontSize: 11, color: "#64748b" }}>
        {isFunction ? `[${agent.language}]` : "prompt"}
      </div>
      <div
        style={{
          fontSize: 11,
          color: "#94a3b8",
          marginTop: 4,
          maxWidth: 200,
          overflow: "hidden",
          textOverflow: "ellipsis",
          whiteSpace: "nowrap",
        }}
      >
        {agent.content.slice(0, 60)}
        {agent.content.length > 60 ? "..." : ""}
      </div>
      <Handle type="source" position={Position.Right} />
    </div>
  );
}

export function FunctionNode({ data }: AgentNodeProps) {
  return <AgentNode data={data} />;
}
