import { Handle, Position } from "@xyflow/react";

interface ExternalInputNodeProps {
  data: { name: string };
}

export function ExternalInputNode({ data }: ExternalInputNodeProps) {
  return (
    <div
      style={{
        padding: "6px 12px",
        border: "1px dashed #94a3b8",
        borderRadius: 999,
        background: "#f1f5f9",
        color: "#475569",
        fontSize: 12,
        fontFamily: "system-ui, sans-serif",
        display: "flex",
        alignItems: "center",
        gap: 6,
        minWidth: 60,
      }}
    >
      <svg
        width="11"
        height="13"
        viewBox="0 0 11 13"
        fill="none"
        stroke="#64748b"
        strokeWidth="1"
        strokeLinejoin="round"
      >
        <path d="M1 1h5l4 4v7H1z" />
        <path d="M6 1v4h4" />
      </svg>
      <span style={{ fontWeight: 500 }}>{data.name}</span>
      <Handle
        id="data-out"
        type="source"
        position={Position.Right}
        style={{ opacity: 0 }}
      />
    </div>
  );
}
