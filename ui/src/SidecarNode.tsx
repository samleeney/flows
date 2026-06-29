import { Handle, Position } from "@xyflow/react";

interface LoopColor {
  stroke: string;
  fill: string;
  soft: string;
  badgeBg: string;
  text: string;
}

const DEFAULT_LOOP_COLOR: LoopColor = {
  stroke: "#2563eb",
  fill: "rgba(219, 234, 254, 0.42)",
  soft: "#dbeafe",
  badgeBg: "#dbeafe",
  text: "#1d4ed8",
};

interface SidecarNodeProps {
  data: {
    kind: "input" | "output";
    name: string;
    source?: string;
    sourceKind?: "agent" | "external";
    fallback?: string;
    fallbackKind?: "agent" | "external";
    target?: string;
    inputName?: string;
    isFallback?: boolean;
    isFeedback?: boolean;
    loopColor?: LoopColor;
    condition?: string;
  };
}

interface TerminalNodeProps {
  data: {
    terminalKind: "start" | "end";
    label: string;
    description?: string;
  };
}

interface LoopFrameNodeProps {
  data: {
    label: string;
    condition?: string;
    source?: string;
    target?: string;
    width?: number;
    height?: number;
    depth?: number;
    loopColor?: LoopColor;
  };
}

interface GoalNodeProps {
  data: {
    kind: "goal";
    target: string;
    objective: string;
    validation?: string[];
    maxTurns?: number;
    tokenBudget?: number;
    onExhaustion?: string;
  };
}

function sourceTone(kind?: "agent" | "external") {
  return kind === "external"
    ? { bg: "#eff6ff", border: "#bfdbfe", color: "#1d4ed8" }
    : { bg: "#f0fdfa", border: "#99f6e4", color: "#0f766e" };
}

function SourcePill({
  value,
  kind,
}: {
  value?: string;
  kind?: "agent" | "external";
}) {
  if (!value) return null;
  const colors = sourceTone(kind);
  return (
    <span
      title={value}
      style={{
        maxWidth: 92,
        overflow: "hidden",
        textOverflow: "ellipsis",
        whiteSpace: "nowrap",
        borderRadius: 4,
        border: `1px solid ${colors.border}`,
        background: colors.bg,
        color: colors.color,
        padding: "1px 5px",
        fontWeight: 700,
      }}
    >
      {value}
    </span>
  );
}

export function InputNode({ data }: SidecarNodeProps) {
  return (
    <div
      title={`${data.name} input for ${data.target}`}
      style={{
        width: 180,
        boxSizing: "border-box",
        padding: "6px 8px",
        border: "1px solid #94a3b8",
        borderRadius: 6,
        background: "#f8fafc",
        color: "#334155",
        fontFamily: "system-ui, sans-serif",
        cursor: "pointer",
      }}
    >
      <div style={{ display: "flex", gap: 6, alignItems: "center" }}>
        <span
          style={{
            borderRadius: 4,
            background: "#e2e8f0",
            color: "#475569",
            padding: "1px 5px",
            fontSize: 9,
            fontWeight: 800,
            letterSpacing: 0,
          }}
        >
          INPUT
        </span>
        <span
          style={{
            minWidth: 0,
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
            fontSize: 12,
            fontWeight: 800,
          }}
        >
          {data.name}
        </span>
      </div>
      <div
        style={{
          display: "flex",
          gap: 4,
          alignItems: "center",
          marginTop: 5,
          minWidth: 0,
          fontSize: 10,
          color: "#64748b",
        }}
      >
        <span>from</span>
        <SourcePill value={data.source} kind={data.sourceKind} />
        {data.fallback && (
          <>
            <span>else</span>
            <SourcePill value={data.fallback} kind={data.fallbackKind} />
          </>
        )}
      </div>
      <Handle
        id="data-in"
        type="target"
        position={Position.Left}
        style={{ opacity: 0 }}
      />
      <Handle
        id="data-out"
        type="source"
        position={Position.Right}
        style={{ opacity: 0 }}
      />
    </div>
  );
}

export function OutputNode({ data }: SidecarNodeProps) {
  const isConditional = Boolean(data.condition);
  const isLoop = Boolean(data.isFeedback);
  const loopTone = data.loopColor ?? DEFAULT_LOOP_COLOR;
  const border = isLoop ? loopTone.stroke : isConditional ? "#111827" : "#5eead4";
  const background = isLoop
    ? loopTone.fill
    : isConditional
    ? "#f8fafc"
    : "#f0fdfa";
  const badgeBg = isLoop
    ? loopTone.badgeBg
    : isConditional
    ? "#e5e7eb"
    : "#ccfbf1";
  const badgeColor = isLoop
    ? loopTone.text
    : isConditional
    ? "#111827"
    : "#0f766e";
  return (
    <div
      title={`${data.name} output from ${data.source ?? "block"}`}
      style={{
        width: 170,
        boxSizing: "border-box",
        padding: "6px 8px",
        border: `${isLoop ? 2 : 1}px solid ${border}`,
        borderRadius: 6,
        background,
        color: "#334155",
        fontFamily: "system-ui, sans-serif",
        cursor: "pointer",
      }}
    >
      <div style={{ display: "flex", gap: 6, alignItems: "center" }}>
        <span
          style={{
            borderRadius: 4,
            background: badgeBg,
            color: badgeColor,
            padding: "1px 5px",
            fontSize: 9,
            fontWeight: 800,
            letterSpacing: 0,
          }}
        >
          {isLoop ? "LOOP" : "OUTPUT"}
        </span>
        <span
          style={{
            minWidth: 0,
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
            fontSize: 12,
            fontWeight: 800,
          }}
        >
          {data.name}
        </span>
      </div>
      <div
        style={{
          marginTop: 5,
          fontSize: 10,
          color: "#64748b",
          overflow: "hidden",
          textOverflow: "ellipsis",
          whiteSpace: "nowrap",
        }}
      >
        to {data.target}
        {data.inputName ? `:${data.inputName}` : ""}
      </div>
      {data.condition && (
        <div
          style={{
            marginTop: 3,
            fontSize: 10,
            color: isLoop ? loopTone.text : "#111827",
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
            fontWeight: 700,
          }}
        >
          {data.condition}
        </div>
      )}
      <Handle
        id="data-in"
        type="target"
        position={Position.Left}
        style={{ top: "42%", opacity: 0 }}
      />
      <Handle
        id="data-out"
        type="source"
        position={Position.Right}
        style={{ top: "42%", opacity: 0 }}
      />
      <Handle
        id="ctrl-out"
        type="source"
        position={Position.Right}
        style={{ top: "70%", opacity: 0 }}
      />
    </div>
  );
}

export function GoalNode({ data }: GoalNodeProps) {
  const validationCount = data.validation?.length ?? 0;
  return (
    <div
      title={data.objective}
      style={{
        width: 210,
        boxSizing: "border-box",
        padding: "8px 10px",
        border: "2px solid #7c3aed",
        borderRadius: 6,
        background: "#f5f3ff",
        color: "#312e81",
        fontFamily: "system-ui, sans-serif",
        cursor: "pointer",
      }}
    >
      <div style={{ display: "flex", gap: 6, alignItems: "center" }}>
        <span
          style={{
            borderRadius: 4,
            background: "#ede9fe",
            color: "#5b21b6",
            padding: "1px 6px",
            fontSize: 9,
            fontWeight: 900,
            letterSpacing: 0,
          }}
        >
          GOAL
        </span>
        <span
          style={{
            minWidth: 0,
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
            fontSize: 12,
            fontWeight: 800,
          }}
        >
          {data.target}
        </span>
      </div>
      <div
        style={{
          marginTop: 6,
          fontSize: 11,
          lineHeight: "14px",
          color: "#4338ca",
          overflow: "hidden",
          display: "-webkit-box",
          WebkitLineClamp: 2,
          WebkitBoxOrient: "vertical",
        }}
      >
        {data.objective}
      </div>
      <div
        style={{
          display: "flex",
          gap: 8,
          marginTop: 6,
          fontSize: 10,
          color: "#6d28d9",
          fontWeight: 700,
          whiteSpace: "nowrap",
          overflow: "hidden",
          textOverflow: "ellipsis",
        }}
      >
        {validationCount > 0 && <span>{validationCount} checks</span>}
        {data.maxTurns ? <span>max {data.maxTurns} turns</span> : null}
      </div>
      <Handle
        id="goal-out"
        type="source"
        position={Position.Bottom}
        style={{ opacity: 0 }}
      />
    </div>
  );
}

export function TerminalNode({ data }: TerminalNodeProps) {
  const isStart = data.terminalKind === "start";
  const colors = isStart
    ? { border: "#22c55e", bg: "#dcfce7", color: "#166534" }
    : { border: "#64748b", bg: "#f1f5f9", color: "#334155" };
  return (
    <div
      title={data.description}
      style={{
        width: 116,
        height: 54,
        boxSizing: "border-box",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        border: `2px solid ${colors.border}`,
        borderRadius: 999,
        background: colors.bg,
        color: colors.color,
        fontFamily: "system-ui, sans-serif",
        fontSize: 13,
        fontWeight: 900,
        letterSpacing: 0,
      }}
    >
      {data.label}
      {isStart ? (
        <Handle
          id="ctrl-out"
          type="source"
          position={Position.Right}
          style={{ opacity: 0 }}
        />
      ) : (
        <Handle
          id="ctrl-in"
          type="target"
          position={Position.Left}
          style={{ opacity: 0 }}
        />
      )}
    </div>
  );
}

export function LoopFrameNode({ data }: LoopFrameNodeProps) {
  const width = data.width ?? 360;
  const height = data.height ?? 260;
  const depth = data.depth ?? 0;
  const loopTone = data.loopColor ?? DEFAULT_LOOP_COLOR;
  const returnY = Math.max(64, height - 38);
  const returnStartX = Math.max(120, width - 34);
  const returnEndX = 38;
  return (
    <div
      title={data.condition}
      style={{
        width,
        height,
        boxSizing: "border-box",
        border: `2px solid ${loopTone.stroke}`,
        borderRadius: 8,
        background: loopTone.fill,
        color: loopTone.text,
        fontFamily: "system-ui, sans-serif",
        pointerEvents: "none",
        position: "relative",
        boxShadow:
          depth > 0 ? `inset 0 0 0 1px ${loopTone.soft}` : undefined,
      }}
    >
      <svg
        aria-hidden="true"
        width={width}
        height={height}
        viewBox={`0 0 ${width} ${height}`}
        style={{
          position: "absolute",
          inset: 0,
          overflow: "visible",
        }}
      >
        <path
          d={`M ${returnStartX} ${returnY} H ${returnEndX}`}
          fill="none"
          stroke={loopTone.stroke}
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={4.5}
        />
        <path
          d={`M ${returnEndX} ${returnY} l 12 -7 M ${returnEndX} ${returnY} l 12 7`}
          fill="none"
          stroke={loopTone.stroke}
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={4.5}
        />
      </svg>
      <div
        style={{
          position: "absolute",
          top: 8,
          left: 10,
          maxWidth: Math.max(180, width - 24),
          display: "flex",
          alignItems: "center",
          gap: 8,
          padding: "3px 6px",
          border: `1px solid ${loopTone.soft}`,
          borderRadius: 6,
          background: "rgba(255, 255, 255, 0.96)",
          fontSize: 11,
          whiteSpace: "nowrap",
          overflow: "hidden",
          textOverflow: "ellipsis",
          boxSizing: "border-box",
        }}
      >
        <span
          style={{
            flexShrink: 0,
            border: `1px solid ${loopTone.soft}`,
            borderRadius: 4,
            background: loopTone.badgeBg,
            color: loopTone.text,
            padding: "1px 6px",
            fontWeight: 900,
            letterSpacing: 0,
          }}
        >
          {data.label}
        </span>
        <span
          style={{
            minWidth: 0,
            overflow: "hidden",
            textOverflow: "ellipsis",
            fontWeight: 800,
          }}
        >
          {data.condition}
        </span>
      </div>
    </div>
  );
}
