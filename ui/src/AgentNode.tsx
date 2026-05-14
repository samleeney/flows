import { Handle, Position } from "@xyflow/react";
import type { AgentJSON, AgentLiveState, AgentLiveStatus } from "./types";

interface AgentNodeProps {
  data: { agent: AgentJSON; label: string; liveState?: AgentLiveState };
}

interface ColorScheme {
  border: string;
  background: string;
  badgeBg: string;
}

function colorsFor(status: AgentLiveStatus, isFunction: boolean): ColorScheme {
  switch (status) {
    case "running":
      return {
        border: "#f59e0b",
        background: "#fef3c7",
        badgeBg: "#f59e0b",
      };
    case "done":
      return {
        border: "#10b981",
        background: "#d1fae5",
        badgeBg: "#10b981",
      };
    case "failed":
      return {
        border: "#ef4444",
        background: "#fee2e2",
        badgeBg: "#ef4444",
      };
    case "idle":
    default:
      return isFunction
        ? { border: "#6366f1", background: "#eef2ff", badgeBg: "#6366f1" }
        : { border: "#3b82f6", background: "#eff6ff", badgeBg: "#3b82f6" };
  }
}

function formatLabel(
  base: string,
  status: AgentLiveStatus,
  iter: number
): string {
  if (status === "running" && iter > 0) {
    return `${base} ▶${iter}`;
  }
  if ((status === "done" || status === "failed") && iter > 1) {
    return `${base} ×${iter}`;
  }
  return base;
}

export function AgentNode({ data }: AgentNodeProps) {
  const { agent, label, liveState } = data;
  const isFunction = agent.node_type === "function";
  const status: AgentLiveStatus = liveState?.status ?? "idle";
  const iter = liveState?.iter ?? 0;
  const colors = colorsFor(status, isFunction);
  const labelDisplay = formatLabel(label, status, iter);

  return (
    <div
      style={{
        padding: "12px 16px",
        border: `2px solid ${colors.border}`,
        borderRadius: isFunction ? "12px" : "6px",
        background: colors.background,
        minWidth: 160,
        fontFamily: "system-ui, sans-serif",
        transition: "background 200ms ease, border-color 200ms ease",
      }}
    >
      <Handle
        id="ctrl-in"
        type="target"
        position={Position.Left}
        style={{ top: "32%", opacity: 0 }}
      />
      <Handle
        id="data-in"
        type="target"
        position={Position.Left}
        style={{ top: "68%", opacity: 0 }}
      />
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 8,
          marginBottom: 4,
        }}
      >
        <span
          style={{
            fontFamily:
              "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
            fontSize: 11,
            fontWeight: 700,
            padding: "2px 6px",
            borderRadius: 4,
            background: colors.badgeBg,
            color: "white",
            letterSpacing: 0.5,
          }}
        >
          {labelDisplay}
        </span>
        <span style={{ fontWeight: 600 }}>{agent.name}</span>
        {status === "running" && (
          <span className="flow-spinner" style={{ marginLeft: "auto" }} />
        )}
      </div>
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
      {liveState?.error && (
        <div
          style={{
            fontSize: 10,
            color: "#b91c1c",
            marginTop: 4,
            maxWidth: 200,
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
          title={liveState.error}
        >
          {liveState.error}
        </div>
      )}
      <Handle
        id="ctrl-out"
        type="source"
        position={Position.Right}
        style={{ top: "32%", opacity: 0 }}
      />
      <Handle
        id="data-out"
        type="source"
        position={Position.Right}
        style={{ top: "68%", opacity: 0 }}
      />
    </div>
  );
}

export function FunctionNode({ data }: AgentNodeProps) {
  return <AgentNode data={data} />;
}
