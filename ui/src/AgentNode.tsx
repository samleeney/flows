import { Handle, Position } from "@xyflow/react";
import type { AgentJSON, AgentLiveState, AgentLiveStatus } from "./types";

interface AgentNodeProps {
  data: {
    agent: AgentJSON;
    label: string;
    liveState?: AgentLiveState;
    role?: AgentRole;
  };
}

interface ColorScheme {
  border: string;
  background: string;
  badgeBg: string;
}

interface AgentRole {
  isStart?: boolean;
  isFinish?: boolean;
  isStopGate?: boolean;
}

interface RoleBadgeProps {
  label: "START" | "END" | "GOAL";
  title: string;
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

function typeColors(isFunction: boolean) {
  return isFunction
    ? { bg: "#3730a3", color: "#eef2ff", border: "#a5b4fc" }
    : { bg: "#1d4ed8", color: "#eff6ff", border: "#93c5fd" };
}

function roleColors(label: RoleBadgeProps["label"]) {
  switch (label) {
    case "START":
      return { bg: "#dcfce7", border: "#86efac", color: "#166534" };
    case "GOAL":
      return { bg: "#ede9fe", border: "#c4b5fd", color: "#5b21b6" };
    case "END":
    default:
      return { bg: "#ffe4e6", border: "#fda4af", color: "#9f1239" };
  }
}

function RoleBadge({ label, title }: RoleBadgeProps) {
  const colors = roleColors(label);
  return (
    <span
      title={title}
      style={{
        border: `1px solid ${colors.border}`,
        borderRadius: 4,
        background: colors.bg,
        color: colors.color,
        fontSize: 10,
        fontWeight: 800,
        lineHeight: "14px",
        padding: "1px 5px",
        letterSpacing: 0.4,
      }}
    >
      {label}
    </span>
  );
}

function TypeGlyph({
  isFunction,
  language,
}: {
  isFunction: boolean;
  language?: string;
}) {
  const colors = typeColors(isFunction);
  const sharedStyle = {
    width: 24,
    height: 24,
    flexShrink: 0,
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    border: `1px solid ${colors.border}`,
    background: colors.bg,
    color: colors.color,
  };
  if (!isFunction) {
    return (
      <span
        title="agent prompt"
        style={{
          ...sharedStyle,
          borderRadius: 999,
          fontSize: 18,
          fontWeight: 800,
          lineHeight: "20px",
        }}
      >
        ∞
      </span>
    );
  }

  return (
    <span
      title={`${language ?? "code"} code block`}
      style={{
        ...sharedStyle,
        borderRadius: 6,
        fontFamily: isFunction
          ? "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace"
          : "system-ui, sans-serif",
        fontSize: isFunction ? 11 : 12,
        fontWeight: 800,
      }}
    >
      {"{}"}
    </span>
  );
}

export function AgentNode({ data }: AgentNodeProps) {
  const { agent, label, liveState, role } = data;
  const isFunction = agent.node_type === "function";
  const status: AgentLiveStatus = liveState?.status ?? "idle";
  const iter = liveState?.iter ?? 0;
  const colors = colorsFor(status, isFunction);
  const labelDisplay = formatLabel(label, status, iter);
  const hasRoleBadge =
    role?.isStart || role?.isFinish || role?.isStopGate || agent.goal;

  return (
    <div
      style={{
        padding: "12px 16px",
        border: `2px solid ${colors.border}`,
        borderRadius: isFunction ? "12px" : "6px",
        background: colors.background,
        width: 280,
        boxSizing: "border-box",
        fontFamily: "system-ui, sans-serif",
        cursor: "pointer",
        transition: "background 200ms ease, border-color 200ms ease",
        boxShadow: role?.isStart
          ? "inset 5px 0 0 #22c55e"
          : role?.isFinish || role?.isStopGate
          ? "inset -5px 0 0 #fb7185"
          : undefined,
      }}
    >
      <Handle
        id="goal-in"
        type="target"
        position={Position.Top}
        style={{ opacity: 0 }}
      />
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
        <TypeGlyph isFunction={isFunction} language={agent.language} />
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
        <span
          title={agent.name}
          style={{
            fontWeight: 600,
            minWidth: 0,
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
        >
          {agent.name}
        </span>
        {status === "running" && (
          <span className="flow-spinner" style={{ marginLeft: "auto" }} />
        )}
      </div>
      <div style={{ fontSize: 11, color: "#64748b" }}>
        {isFunction ? `${agent.language ?? "code"} code block` : "agent prompt"}
      </div>
      {hasRoleBadge && (
        <div
          style={{
            display: "flex",
            gap: 5,
            marginTop: 6,
            flexWrap: "wrap",
          }}
        >
          {role?.isStart && (
            <RoleBadge label="START" title="Runs from an always condition" />
          )}
          {agent.goal && (
            <RoleBadge label="GOAL" title={agent.goal.objective} />
          )}
          {(role?.isFinish || role?.isStopGate) && (
            <RoleBadge
              label="END"
              title={
                role?.isStopGate
                  ? "The run ends here when no downstream condition matches"
                  : "No downstream agent starts from this output"
              }
            />
          )}
        </div>
      )}
      <div
        style={{
          fontSize: 11,
          color: "#94a3b8",
          marginTop: hasRoleBadge ? 6 : 4,
          maxWidth: 240,
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
            maxWidth: 240,
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
