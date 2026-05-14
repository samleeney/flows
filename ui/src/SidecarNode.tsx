import { Handle, Position } from "@xyflow/react";

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
    condition?: string;
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
            letterSpacing: 0.4,
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
  return (
    <div
      title={`${data.name} output from ${data.source ?? "block"}`}
      style={{
        width: 170,
        boxSizing: "border-box",
        padding: "6px 8px",
        border: `1px solid ${isConditional ? "#fb7185" : "#5eead4"}`,
        borderRadius: 6,
        background: isConditional ? "#fff1f2" : "#f0fdfa",
        color: "#334155",
        fontFamily: "system-ui, sans-serif",
        cursor: "pointer",
      }}
    >
      <div style={{ display: "flex", gap: 6, alignItems: "center" }}>
        <span
          style={{
            borderRadius: 4,
            background: isConditional ? "#ffe4e6" : "#ccfbf1",
            color: isConditional ? "#be123c" : "#0f766e",
            padding: "1px 5px",
            fontSize: 9,
            fontWeight: 800,
            letterSpacing: 0.4,
          }}
        >
          OUTPUT
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
            color: "#be123c",
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
