import {
  useState,
  useCallback,
  useMemo,
  useRef,
  useEffect,
  type ReactNode,
} from "react";
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
  ExternalInputOrigin,
  FlowJSON,
  LiveUIState,
  RunRecord,
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
  GoalNode,
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
  goalNode: GoalNode,
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
    return decorateNodes(nodes, run?.agents, run?.external_inputs);
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
          flow={flow}
          node={selectedNode}
          selectedRun={selectedRun}
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
  flow,
  node,
  selectedRun,
  onClose,
}: {
  flow: FlowJSON;
  node: Node;
  selectedRun?: RunRecord;
  onClose: () => void;
}) {
  const data = node.data as Record<string, unknown>;
  const agent = data.agent as FlowJSON["agents"][number] | undefined;
  const kind = data.kind as string | undefined;
  const title = agent?.name ?? String(data.name ?? data.label ?? node.id);
  const origins = externalInputOriginMap(selectedRun);
  const detailText = formatNodeDetailsText(flow, node, origins);

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
        <strong style={{ fontSize: 13 }}>{title}</strong>
        <div style={{ display: "flex", gap: 6, alignItems: "center" }}>
          <button
            type="button"
            onClick={() => openDetailsWindow(title, detailText)}
            style={{
              border: "1px solid #cbd5e1",
              borderRadius: 4,
              background: "#f8fafc",
              color: "#334155",
              cursor: "pointer",
              fontSize: 11,
              fontWeight: 700,
              lineHeight: "16px",
              padding: "2px 7px",
            }}
            aria-label="Open details in a new window"
            title="Open details in a new window"
          >
            Open
          </button>
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
      </div>
      <div style={{ padding: 12, display: "grid", gap: 10, fontSize: 12 }}>
        {agent ? <AgentDetails agent={agent} data={data} origins={origins} /> : null}
        {kind === "input" ? <InputDetails data={data} origins={origins} /> : null}
        {kind === "output" ? <OutputDetails data={data} /> : null}
        {kind === "goal" ? <GoalDetails data={data} /> : null}
        {kind === "terminal" ? <TerminalDetails data={data} /> : null}
      </div>
    </div>
  );
}

function DetailRow({ label, value }: { label: string; value?: unknown }) {
  if (!hasDetailValue(value)) return null;
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

function hasDetailValue(value: unknown) {
  return value !== undefined && value !== null && value !== "";
}

type GoalParts = {
  objective?: unknown;
  validation: string[];
  maxTurns?: unknown;
  tokenBudget?: unknown;
  onExhaustion?: unknown;
};

type InputSummary = {
  name: string;
  source: string;
  sourceKind?: "agent" | "external";
  sourceBlockLabel?: string;
  fallback?: string;
  fallbackKind?: "agent" | "external";
  fallbackBlockLabel?: string;
  externalOrigin?: ExternalInputOrigin;
};

function externalInputOriginMap(run?: RunRecord): Map<string, ExternalInputOrigin> {
  return new Map((run?.external_inputs ?? []).map((origin) => [origin.name, origin]));
}

function withInputOrigins(
  inputs: InputSummary[] | undefined,
  origins: Map<string, ExternalInputOrigin>
): InputSummary[] | undefined {
  return inputs?.map((input) => {
    if (input.sourceKind !== "external" && input.fallbackKind !== "external") {
      return input;
    }
    return {
      ...input,
      externalOrigin: origins.get(input.name),
    };
  });
}

function GoalSections({ goal }: { goal: GoalParts }) {
  return (
    <section
      style={{
        borderLeft: "3px solid #7c3aed",
        padding: "2px 0 2px 12px",
        background:
          "linear-gradient(90deg, rgba(124, 58, 237, 0.08), rgba(255, 255, 255, 0) 82%)",
      }}
    >
      <div
        style={{
          color: "#5b21b6",
          fontSize: 10,
          fontWeight: 800,
          lineHeight: "14px",
          textTransform: "uppercase",
        }}
      >
        goal
      </div>
      <div style={{ display: "grid", gap: 10, marginTop: 8 }}>
        <GoalSection title="Objective" value={goal.objective} />
        {goal.validation.length ? (
          <GoalSection title="Validation">
            <ValidationList items={goal.validation} />
          </GoalSection>
        ) : null}
        <GoalSection title="Max turns" value={goal.maxTurns} />
        <GoalSection title="Token budget" value={goal.tokenBudget} />
        <GoalSection title="On exhaustion" value={goal.onExhaustion} />
      </div>
    </section>
  );
}

function GoalSection({
  title,
  value,
  children,
}: {
  title: string;
  value?: unknown;
  children?: ReactNode;
}) {
  if (!hasDetailValue(value) && children === undefined) return null;
  return (
    <div>
      <div
        style={{
          color: "#475569",
          fontSize: 11,
          fontWeight: 800,
          lineHeight: "15px",
        }}
      >
        {title}
      </div>
      {children ?? (
        <div
          style={{
            color: "#1f2937",
            lineHeight: 1.45,
            marginTop: 3,
            overflowWrap: "anywhere",
          }}
        >
          {String(value)}
        </div>
      )}
    </div>
  );
}

function ValidationList({ items }: { items: string[] }) {
  return (
    <ol
      style={{
        display: "grid",
        gap: 6,
        listStyle: "none",
        margin: "4px 0 0",
        padding: 0,
      }}
    >
      {items.map((item, index) => (
        <li
          key={`${index}-${item}`}
          style={{
            alignItems: "start",
            columnGap: 8,
            display: "grid",
            gridTemplateColumns: "20px minmax(0, 1fr)",
          }}
        >
          <span
            aria-hidden="true"
            style={{
              alignItems: "center",
              background: "#ede9fe",
              border: "1px solid #c4b5fd",
              borderRadius: 999,
              color: "#5b21b6",
              display: "inline-flex",
              fontSize: 10,
              fontWeight: 800,
              height: 18,
              justifyContent: "center",
              lineHeight: "18px",
              marginTop: 1,
              width: 18,
            }}
          >
            {index + 1}
          </span>
          <span
            style={{
              color: "#1f2937",
              lineHeight: 1.45,
              overflowWrap: "anywhere",
            }}
          >
            {item}
          </span>
        </li>
      ))}
    </ol>
  );
}

function AuthoredPromptSection({ content }: { content: string }) {
  return (
    <div>
      <div style={{ fontSize: 10, color: "#94a3b8", fontWeight: 700 }}>
        authored prompt
      </div>
      <pre
        style={{
          background: "#f8fafc",
          border: "1px solid #e2e8f0",
          borderRadius: 6,
          color: "#1f2937",
          fontFamily:
            "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
          fontSize: 11,
          lineHeight: 1.45,
          margin: "4px 0 0",
          maxHeight: 260,
          overflow: "auto",
          overflowWrap: "anywhere",
          padding: 9,
          whiteSpace: "pre-wrap",
        }}
      >
        {content}
      </pre>
    </div>
  );
}

function InputsSection({
  inputs,
  target,
}: {
  inputs?: InputSummary[];
  target: string;
}) {
  if (!inputs?.length) return null;
  return (
    <section>
      <div style={{ fontSize: 10, color: "#94a3b8", fontWeight: 700 }}>
        inputs
      </div>
      <div style={{ display: "grid", gap: 7, marginTop: 5 }}>
        {inputs.map((input) => (
          <button
            key={input.name}
            type="button"
            onClick={() => openInputVisualizer(target, input)}
            title={`Open ${input.name} input visualizer`}
            style={{
              background: "#ffffff",
              border: "1px solid #cbd5e1",
              borderLeft: "3px solid #0d9488",
              borderRadius: 6,
              color: "#334155",
              cursor: "pointer",
              display: "grid",
              gap: 5,
              padding: "7px 8px",
              textAlign: "left",
              width: "100%",
            }}
          >
            <span
              style={{
                alignItems: "center",
                display: "flex",
                gap: 6,
                minWidth: 0,
              }}
            >
              <span
                style={{
                  background: "#f0fdfa",
                  border: "1px solid #99f6e4",
                  borderRadius: 4,
                  color: "#0f766e",
                  fontSize: 9,
                  fontWeight: 800,
                  lineHeight: "13px",
                  padding: "1px 5px",
                }}
              >
                INPUT
              </span>
              <span
                style={{
                  color: "#0f172a",
                  fontSize: 12,
                  fontWeight: 800,
                  minWidth: 0,
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  whiteSpace: "nowrap",
                }}
              >
                {input.name}
              </span>
              <span
                aria-hidden="true"
                style={{ color: "#94a3b8", fontSize: 13, marginLeft: "auto" }}
              >
                ↗
              </span>
            </span>
            <span
              style={{
                alignItems: "center",
                color: "#64748b",
                display: "flex",
                flexWrap: "wrap",
                gap: 4,
                fontSize: 11,
                lineHeight: "16px",
              }}
            >
              <span>from</span>
              <SourceChip
                value={inputSourceDisplay(input, "source")}
                kind={input.sourceKind}
              />
              {input.fallback ? (
                <>
                  <span>else</span>
                  <SourceChip
                    value={inputSourceDisplay(input, "fallback")}
                    kind={input.fallbackKind}
                  />
                </>
              ) : null}
            </span>
            {inputHasExternalSource(input) ? (
              <span
                style={{
                  color: "#64748b",
                  fontSize: 10,
                  lineHeight: "14px",
                  overflowWrap: "anywhere",
                }}
              >
                {externalOriginSummary(input)}
              </span>
            ) : null}
          </button>
        ))}
      </div>
    </section>
  );
}

function inputHasExternalSource(input: InputSummary) {
  return input.sourceKind === "external" || input.fallbackKind === "external";
}

function inputSourceDisplay(
  input: InputSummary,
  role: "source" | "fallback"
) {
  const kind = role === "source" ? input.sourceKind : input.fallbackKind;
  const value = role === "source" ? input.source : input.fallback;
  const blockLabel =
    role === "source" ? input.sourceBlockLabel : input.fallbackBlockLabel;
  if (kind !== "external") {
    return blockLabel ? `output ${blockLabel}` : value;
  }

  const origin = input.externalOrigin;
  if (origin?.source === "file") {
    return origin.file_name ?? "file input";
  }
  if (origin?.source === "inline") {
    return "inline input";
  }
  return "external input";
}

function externalOriginSummary(input: InputSummary) {
  const origin = input.externalOrigin;
  if (!origin) {
    return "External value is supplied when the flow runs; this static chart has no file/value attached.";
  }
  if (origin.source === "file") {
    const name = origin.file_name ?? origin.path ?? input.name;
    const path = origin.path && origin.path !== name ? ` · ${origin.path}` : "";
    return `File: ${name}${path}${formatByteSuffix(origin.bytes)}`;
  }
  return `Inline value${formatByteSuffix(origin.bytes)}`;
}

function formatByteSuffix(bytes?: number) {
  return typeof bytes === "number" && bytes > 0 ? ` · ${formatBytes(bytes)}` : "";
}

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function SourceChip({
  value,
  kind,
}: {
  value?: string;
  kind?: "agent" | "external";
}) {
  if (!value) return null;
  const colors =
    kind === "external"
      ? { bg: "#eff6ff", border: "#bfdbfe", color: "#1d4ed8" }
      : { bg: "#f0fdfa", border: "#99f6e4", color: "#0f766e" };
  return (
    <span
      style={{
        background: colors.bg,
        border: `1px solid ${colors.border}`,
        borderRadius: 4,
        color: colors.color,
        fontWeight: 700,
        maxWidth: 126,
        overflow: "hidden",
        padding: "0 5px",
        textOverflow: "ellipsis",
        whiteSpace: "nowrap",
      }}
    >
      {value}
    </span>
  );
}

function AgentDetails({
  agent,
  data,
  origins,
}: {
  agent: FlowJSON["agents"][number];
  data: Record<string, unknown>;
  origins: Map<string, ExternalInputOrigin>;
}) {
  const inputs = withInputOrigins(data.inputs as InputSummary[] | undefined, origins);
  if (agent.node_type === "prompt") {
    return (
      <>
        {agent.goal ? <GoalSections goal={goalPartsFromGoal(agent.goal)} /> : null}
        <InputsSection inputs={inputs} target={agent.name} />
        <AuthoredPromptSection content={agent.content} />
      </>
    );
  }

  return (
    <>
      <DetailRow
        label="type"
        value={
          `${agent.language ?? "code"} code block`
        }
      />
      <InputsSection inputs={inputs} target={agent.name} />
      <DetailRow
        label="starts"
        value={agent.start
          .map((start) => formatStartCondition(start))
          .join("\n")}
      />
      <DetailRow
        label="code"
        value={agent.content}
      />
    </>
  );
}

function formatGoal(goal: NonNullable<FlowJSON["agents"][number]["goal"]>) {
  return formatGoalParts(goalPartsFromGoal(goal));
}

function goalPartsFromGoal(
  goal: NonNullable<FlowJSON["agents"][number]["goal"]>
): GoalParts {
  return {
    objective: goal.objective,
    validation: goal.validation ?? [],
    maxTurns: goal.max_turns,
    tokenBudget: goal.token_budget,
    onExhaustion: goal.on_exhaustion,
  };
}

function goalPartsFromData(data: Record<string, unknown>): GoalParts {
  return {
    objective: data.objective,
    validation: Array.isArray(data.validation)
      ? data.validation.map(String)
      : [],
    maxTurns: data.maxTurns,
    tokenBudget: data.tokenBudget,
    onExhaustion: data.onExhaustion,
  };
}

function formatGoalParts({
  objective,
  validation,
  maxTurns,
  tokenBudget,
  onExhaustion,
}: GoalParts) {
  const lines: string[] = [];
  appendGoalTextSection(lines, "Objective", hasDetailValue(objective) ? [String(objective)] : []);
  if (validation.length) {
    appendGoalTextSection(
      lines,
      "Validation",
      validation.map((item, index) => `${index + 1}. ${item}`)
    );
  }
  appendGoalTextSection(lines, "Max turns", hasDetailValue(maxTurns) ? [String(maxTurns)] : []);
  appendGoalTextSection(lines, "Token budget", hasDetailValue(tokenBudget) ? [String(tokenBudget)] : []);
  appendGoalTextSection(lines, "On exhaustion", hasDetailValue(onExhaustion) ? [String(onExhaustion)] : []);
  return lines.join("\n");
}

function appendGoalTextSection(lines: string[], heading: string, body: string[]) {
  if (body.length === 0) return;
  if (lines.length > 0) lines.push("");
  lines.push(heading, ...body);
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

function InputDetails({
  data,
  origins,
}: {
  data: Record<string, unknown>;
  origins: Map<string, ExternalInputOrigin>;
}) {
  const name = String(data.name ?? "");
  const sourceKind = data.sourceKind as string | undefined;
  const fallbackKind = data.fallbackKind as string | undefined;
  const input = inputSummaryFromData(data, origins);
  const target = String(data.target ?? "");
  return (
    <>
      <DetailRow label="kind" value="input" />
      <DetailRow label="input name" value={data.name} />
      <DetailRow label="receiving block" value={data.target} />
      <DetailRow label="source" value={data.source} />
      <DetailRow label="fallback" value={data.fallback} />
      <DetailRow
        label="external source"
        value={inputHasExternalSource(input) ? externalOriginSummary(input) : undefined}
      />
      <DetailRow
        label="runtime value"
        value={formatInputRuntimeValue(name, sourceKind, data.source, input.externalOrigin)}
      />
      <DetailRow
        label="fallback value"
        value={
          data.fallback
            ? formatInputRuntimeValue(name, fallbackKind, data.fallback, input.externalOrigin)
            : undefined
        }
      />
      <button
        type="button"
        onClick={() => openInputVisualizer(target, input)}
        style={{
          background: "#f0fdfa",
          border: "1px solid #99f6e4",
          borderRadius: 6,
          color: "#0f766e",
          cursor: "pointer",
          fontSize: 11,
          fontWeight: 800,
          lineHeight: "16px",
          padding: "6px 8px",
          textAlign: "left",
        }}
      >
        Open input visualizer
      </button>
    </>
  );
}

function inputSummaryFromData(
  data: Record<string, unknown>,
  origins: Map<string, ExternalInputOrigin>
): InputSummary {
  const input: InputSummary = {
    name: String(data.name ?? ""),
    source: String(data.source ?? ""),
    sourceKind: data.sourceKind as "agent" | "external" | undefined,
    sourceBlockLabel: data.sourceBlockLabel
      ? String(data.sourceBlockLabel)
      : undefined,
    fallback: data.fallback ? String(data.fallback) : undefined,
    fallbackKind: data.fallbackKind as "agent" | "external" | undefined,
    fallbackBlockLabel: data.fallbackBlockLabel
      ? String(data.fallbackBlockLabel)
      : undefined,
  };
  return withInputOrigins([input], origins)?.[0] ?? input;
}

function formatInputRuntimeValue(
  name: string,
  sourceKind?: string,
  source?: unknown,
  origin?: ExternalInputOrigin
): string {
  if (sourceKind === "external") {
    if (origin?.source === "file") {
      return `Supplied from file ${origin.file_name ?? origin.path ?? name}${origin.path ? ` at ${origin.path}` : ""}${formatByteSuffix(origin.bytes)}.`;
    }
    if (origin?.source === "inline") {
      return `Supplied inline with --input ${name}=...${formatByteSuffix(origin.bytes)}.`;
    }
    return `Supplied when running the flow with --input ${name}=... or --input ${name}=@file. Static charts do not contain this value.`;
  }
  if (source) {
    return `Resolved at run time from the latest output of ${String(source)}. Static charts show the connection, not the resolved value.`;
  }
  return "Resolved at run time.";
}

function OutputDetails({ data }: { data: Record<string, unknown> }) {
  return (
    <>
      <DetailRow label="kind" value="output" />
      <DetailRow label="summary" value={outputSummary(data)} />
      <DetailRow label="generated by" value={outputBlockLabel(data, "source")} />
      <DetailRow label="goes to" value={outputBlockLabel(data, "target")} />
      <DetailRow label="target input" value={data.inputName} />
      <DetailRow label="condition" value={data.condition} />
    </>
  );
}

function outputSummary(data: Record<string, unknown>) {
  const source = outputBlockLabel(data, "source");
  const target = outputBlockLabel(data, "target");
  if (!source || !target) return undefined;
  if (data.inputName) {
    const prefix = data.isFallback ? "fallback output" : "output";
    return `${prefix} ${source} to ${target} as ${String(data.inputName)}`;
  }
  return `route ${source} starts ${target}`;
}

function outputBlockLabel(
  data: Record<string, unknown>,
  role: "source" | "target"
) {
  const labelKey = role === "source" ? "sourceBlockLabel" : "targetBlockLabel";
  const valueKey = role === "source" ? "source" : "target";
  const label = data[labelKey];
  const value = data[valueKey];
  if (label && value) return `${String(label)} (${String(value)})`;
  if (label) return String(label);
  if (value) return String(value);
  return undefined;
}

function GoalDetails({ data }: { data: Record<string, unknown> }) {
  return (
    <>
      <DetailRow label="kind" value="goal" />
      <DetailRow label="attached block" value={data.target} />
      <GoalSections goal={goalPartsFromData(data)} />
    </>
  );
}

function formatGoalData(data: Record<string, unknown>) {
  return formatGoalParts(goalPartsFromData(data));
}

function formatNodeDetailsText(
  _flow: FlowJSON,
  node: Node,
  origins: Map<string, ExternalInputOrigin>
): string {
  const data = node.data as Record<string, unknown>;
  const agent = data.agent as FlowJSON["agents"][number] | undefined;
  const kind = data.kind as string | undefined;
  const rows: string[] = [];
  const add = (label: string, value?: unknown) => {
    if (value === undefined || value === "") return;
    rows.push(`${label}\n${String(value)}`);
  };

  if (agent) {
    const inputs = withInputOrigins(data.inputs as InputSummary[] | undefined, origins);
    if (agent.node_type === "prompt") {
      if (agent.goal) add("goal", formatGoal(agent.goal));
      add("inputs", formatInputsText(inputs));
      add("authored prompt", agent.content);
      return rows.join("\n\n");
    }

    add("node", agent.name);
    add(
      "type",
      `${agent.language ?? "code"} code block`
    );
    add(
      "inputs",
      formatInputsText(inputs)
    );
    add(
      "starts",
      agent.start.map((start) => formatStartCondition(start)).join("\n")
    );
    add("code", agent.content);
  } else if (kind === "input") {
    add("node", data.name ?? data.label ?? node.id);
    const name = String(data.name ?? "");
    const input = inputSummaryFromData(data, origins);
    add("kind", "input");
    add("input name", data.name);
    add("receiving block", data.target);
    add("source", data.source);
    add("fallback", data.fallback);
    add(
      "runtime value",
      formatInputRuntimeValue(
        name,
        data.sourceKind as string | undefined,
        data.source,
        input.externalOrigin
      )
    );
    if (data.fallback) {
      add(
        "fallback value",
        formatInputRuntimeValue(
          name,
          data.fallbackKind as string | undefined,
          data.fallback,
          input.externalOrigin
        )
      );
    }
  } else if (kind === "output") {
    add("node", data.name ?? data.label ?? node.id);
    add("kind", "output");
    add("summary", outputSummary(data));
    add("generated by", outputBlockLabel(data, "source"));
    add("goes to", outputBlockLabel(data, "target"));
    add("target input", data.inputName);
    add("condition", data.condition);
  } else if (kind === "goal") {
    add("node", data.name ?? data.label ?? node.id);
    add("kind", "goal");
    add("attached block", data.target);
    add("goal", formatGoalData(data));
  } else if (kind === "terminal") {
    add("node", data.name ?? data.label ?? node.id);
    add("kind", data.label);
    add("description", data.description);
  }
  return rows.join("\n\n");
}

function formatInputsText(inputs?: InputSummary[]) {
  return inputs
    ?.map((input) => {
      const route = input.fallback
        ? `${input.name}: ${inputSourceDisplay(input, "source")} else ${inputSourceDisplay(input, "fallback")}`
        : `${input.name}: ${inputSourceDisplay(input, "source")}`;
      return inputHasExternalSource(input)
        ? `${route}\n  ${externalOriginSummary(input)}`
        : route;
    })
    .join("\n");
}

function inputSourceDetail(
  input: InputSummary,
  role: "source" | "fallback"
) {
  const kind = role === "source" ? input.sourceKind : input.fallbackKind;
  if (kind !== "external") return undefined;

  const origin = input.externalOrigin;
  if (origin?.source === "file") {
    return [origin.path, origin.bytes ? formatBytes(origin.bytes) : undefined]
      .filter(Boolean)
      .join(" · ");
  }
  if (origin?.source === "inline") {
    return `supplied inline${formatByteSuffix(origin.bytes)}`;
  }
  return "runtime source not attached to this static chart";
}

function externalInputPreviewHTML(input: InputSummary) {
  const origin = input.externalOrigin;
  if (!origin || origin.preview === undefined) return "";

  const title =
    origin.source === "file"
      ? origin.file_name ?? origin.path ?? input.name
      : `--input ${input.name}=...`;
  const note = origin.preview_truncated
    ? `<div class="preview-note">Preview truncated.</div>`
    : "";
  return `<section class="preview">
      <div class="preview-heading">External input preview</div>
      <div class="preview-meta">${escapeHTML(title)}${escapeHTML(formatByteSuffix(origin.bytes))}</div>
      <pre class="preview-pre">${escapeHTML(origin.preview)}</pre>
      ${note}
    </section>`;
}

function detailsTextToHTML(detailText: string): string {
  return detailText
    .split(/\n{2,}/)
    .map((section) => section.trim())
    .filter(Boolean)
    .map((section) => {
      const [label = "", ...bodyLines] = section.split("\n");
      const body = bodyLines.join("\n");
      if (label === "goal") {
        return `<section class="detail-section goal-detail">
  <div class="detail-label">goal</div>
  ${goalTextToHTML(body)}
</section>`;
      }
      const isCode = label === "authored prompt" || label === "code";
      return `<section class="detail-section">
  <div class="detail-label">${escapeHTML(label)}</div>
  ${
    isCode
      ? `<pre class="code-block">${escapeHTML(body)}</pre>`
      : `<div class="detail-value">${escapeHTML(body)}</div>`
  }
</section>`;
    })
    .join("\n");
}

function goalTextToHTML(goalText: string): string {
  return goalText
    .split(/\n{2,}/)
    .map((section) => section.trim())
    .filter(Boolean)
    .map((section) => {
      const [heading = "", ...bodyLines] = section.split("\n");
      if (heading === "Validation") {
        return `<div class="goal-field">
  <div class="goal-heading">Validation</div>
  <ol class="validation-list">
    ${bodyLines.map(validationLineToHTML).join("\n")}
  </ol>
</div>`;
      }
      return `<div class="goal-field">
  <div class="goal-heading">${escapeHTML(heading)}</div>
  <div class="goal-value">${escapeHTML(bodyLines.join("\n"))}</div>
</div>`;
    })
    .join("\n");
}

function validationLineToHTML(line: string): string {
  const match = line.match(/^(\d+)\.\s*(.*)$/);
  const index = match?.[1] ?? "";
  const text = match?.[2] ?? line;
  return `<li>
    <span class="validation-index">${escapeHTML(index)}</span>
    <span>${escapeHTML(text)}</span>
  </li>`;
}

function openInputVisualizer(target: string, input: InputSummary) {
  const popup = window.open("", "_blank", "width=900,height=640,scrollbars=yes");
  if (!popup) {
    window.alert("The browser blocked the input visualizer window.");
    return;
  }
  popup.opener = null;
  const fallbackRoute = input.fallback
    ? `<section class="route-block fallback-route">
        <div class="route-heading">Fallback route</div>
        <div class="route">
          ${inputVisualNode(
            "Fallback source",
            inputSourceDisplay(input, "fallback"),
            input.fallbackKind,
            inputSourceDetail(input, "fallback")
          )}
          <div class="connector">-></div>
          ${inputVisualNode("Input", input.name, "input")}
        </div>
        <p class="hint">Used only when the primary source has no value available for this run.</p>
      </section>`
    : "";

  popup.document.write(`<!doctype html>
<html>
<head>
  <title>${escapeHTML(input.name)} input</title>
  <style>
    body {
      margin: 28px;
      color: #1f2937;
      background: #ffffff;
      font-family: system-ui, sans-serif;
    }
    h1 {
      font-size: 20px;
      margin: 0;
    }
    .subtitle {
      color: #64748b;
      margin: 4px 0 24px;
    }
    .layout {
      display: grid;
      gap: 22px;
      max-width: 980px;
    }
    .route-block {
      border-left: 4px solid #0d9488;
      background: linear-gradient(90deg, rgba(13, 148, 136, 0.08), rgba(255, 255, 255, 0) 82%);
      padding: 4px 0 4px 16px;
    }
    .fallback-route {
      border-left-color: #64748b;
      background: linear-gradient(90deg, rgba(100, 116, 139, 0.08), rgba(255, 255, 255, 0) 82%);
    }
    .route-heading {
      color: #0f766e;
      font-size: 11px;
      font-weight: 800;
      line-height: 16px;
      margin-bottom: 10px;
      text-transform: uppercase;
    }
    .fallback-route .route-heading {
      color: #475569;
    }
    .route {
      align-items: stretch;
      display: grid;
      gap: 12px;
      grid-template-columns: minmax(0, 1fr) 30px minmax(0, 1fr) 30px minmax(0, 1fr);
    }
    .fallback-route .route {
      grid-template-columns: minmax(0, 1fr) 30px minmax(0, 1fr);
      max-width: 640px;
    }
    .node {
      border: 1px solid #cbd5e1;
      border-radius: 7px;
      background: #ffffff;
      min-width: 0;
      padding: 12px;
    }
    .node.source.external {
      border-color: #bfdbfe;
      background: #eff6ff;
    }
    .node.source.agent {
      border-color: #99f6e4;
      background: #f0fdfa;
    }
    .node.input {
      border-color: #99f6e4;
      background: #f8fffd;
      box-shadow: inset 3px 0 0 #0d9488;
    }
    .node.target {
      border-color: #bfdbfe;
      background: #f8fafc;
    }
    .node-label {
      color: #64748b;
      font-size: 10px;
      font-weight: 800;
      line-height: 14px;
      text-transform: uppercase;
    }
    .node-name {
      color: #0f172a;
      font-size: 15px;
      font-weight: 800;
      margin-top: 5px;
      overflow-wrap: anywhere;
    }
    .node-kind {
      color: #64748b;
      font-size: 12px;
      margin-top: 4px;
    }
    .node-detail {
      color: #475569;
      font-size: 11px;
      line-height: 1.35;
      margin-top: 6px;
      overflow-wrap: anywhere;
    }
    .connector {
      align-self: center;
      color: #94a3b8;
      font-weight: 800;
      text-align: center;
    }
    .hint {
      color: #64748b;
      line-height: 1.5;
      margin: 12px 0 0;
    }
    .runtime {
      border-top: 1px solid #e2e8f0;
      padding-top: 18px;
    }
    .runtime h2 {
      color: #475569;
      font-size: 13px;
      margin: 0 0 6px;
    }
    .runtime p {
      line-height: 1.55;
      margin: 0;
    }
    .preview {
      border-top: 1px solid #e2e8f0;
      padding-top: 18px;
    }
    .preview-heading {
      color: #475569;
      font-size: 13px;
      font-weight: 800;
      margin-bottom: 4px;
    }
    .preview-meta {
      color: #64748b;
      font-size: 12px;
      line-height: 1.4;
      margin-bottom: 8px;
      overflow-wrap: anywhere;
    }
    .preview-pre {
      background: #f8fafc;
      border: 1px solid #e2e8f0;
      border-radius: 7px;
      color: #1f2937;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      font-size: 12px;
      line-height: 1.45;
      margin: 0;
      max-height: 260px;
      overflow: auto;
      padding: 12px;
      white-space: pre-wrap;
    }
    .preview-note {
      color: #64748b;
      font-size: 12px;
      margin-top: 6px;
    }
    @media (max-width: 760px) {
      .route,
      .fallback-route .route {
        grid-template-columns: 1fr;
      }
      .connector {
        transform: rotate(90deg);
      }
    }
  </style>
</head>
<body>
  <h1>${escapeHTML(input.name)}</h1>
  <p class="subtitle">Input for ${escapeHTML(target)}</p>
  <main class="layout">
    <section class="route-block">
      <div class="route-heading">Primary route</div>
      <div class="route">
        ${inputVisualNode(
          "Source",
          inputSourceDisplay(input, "source"),
          input.sourceKind,
          inputSourceDetail(input, "source")
        )}
        <div class="connector">-></div>
        ${inputVisualNode("Input", input.name, "input")}
        <div class="connector">-></div>
        ${inputVisualNode("Receiving block", target, "target")}
      </div>
      <p class="hint">${escapeHTML(inputRuntimeExplanation(input))}</p>
    </section>
    ${fallbackRoute}
    ${externalInputPreviewHTML(input)}
    <section class="runtime">
      <h2>Runtime value</h2>
      <p>${escapeHTML(formatInputRuntimeValue(input.name, input.sourceKind, input.source, input.externalOrigin))}</p>
    </section>
  </main>
</body>
</html>`);
  popup.document.close();
}

function inputVisualNode(
  label: string,
  value?: string,
  kind?: "agent" | "external" | "input" | "target",
  detail?: string
) {
  return `<div class="node ${kind === "external" || kind === "agent" ? `source ${kind}` : kind ?? ""}">
    <div class="node-label">${escapeHTML(label)}</div>
    <div class="node-name">${escapeHTML(value || "(none)")}</div>
    <div class="node-kind">${escapeHTML(inputKindLabel(kind))}</div>
    ${detail ? `<div class="node-detail">${escapeHTML(detail)}</div>` : ""}
  </div>`;
}

function inputKindLabel(kind?: "agent" | "external" | "input" | "target") {
  switch (kind) {
    case "external":
      return "external input";
    case "agent":
      return "latest block output";
    case "input":
      return "named input value";
    case "target":
      return "block receiving this input";
    default:
      return "source";
  }
}

function inputRuntimeExplanation(input: InputSummary) {
  if (input.sourceKind === "external") {
    const origin = input.externalOrigin;
    if (origin?.source === "file") {
      return `The receiving block gets the contents of ${origin.file_name ?? input.name}. It was loaded at run time from ${origin.path ?? "the supplied file path"}.`;
    }
    if (origin?.source === "inline") {
      return `The receiving block gets the inline value supplied as --input ${input.name}=... for this run.`;
    }
    return `This is an external input slot. Run the flow with --input ${input.name}=... or --input ${input.name}=@file to attach the value and source.`;
  }
  if (input.fallbackKind === "external" && input.externalOrigin) {
    return `The primary value is resolved from the latest raw output of ${input.source}. If that is unavailable, the external fallback shown below is used.`;
  }
  return `The value is resolved from the latest raw output of ${input.source}.`;
}

function openDetailsWindow(title: string, detailText: string) {
  const popup = window.open("", "_blank", "width=920,height=760,scrollbars=yes");
  if (!popup) {
    window.alert("The browser blocked the details window.");
    return;
  }
  popup.opener = null;
  popup.document.write(`<!doctype html>
<html>
<head>
  <title>${escapeHTML(title)}</title>
  <style>
    body {
      margin: 24px;
      color: #1f2937;
      background: #ffffff;
      font-family: system-ui, sans-serif;
    }
    h1 {
      font-size: 18px;
      margin: 0 0 16px;
    }
    .detail-list {
      display: grid;
      gap: 18px;
      max-width: 980px;
    }
    .detail-label {
      color: #64748b;
      font-size: 11px;
      font-weight: 800;
      line-height: 16px;
      margin-bottom: 5px;
      text-transform: uppercase;
    }
    .detail-value {
      line-height: 1.5;
      overflow-wrap: anywhere;
      white-space: pre-wrap;
    }
    .goal-detail {
      border-left: 4px solid #7c3aed;
      background: linear-gradient(90deg, rgba(124, 58, 237, 0.08), rgba(255, 255, 255, 0) 82%);
      padding: 2px 0 2px 14px;
    }
    .goal-detail .detail-label {
      color: #5b21b6;
      margin-bottom: 10px;
    }
    .goal-field + .goal-field {
      margin-top: 14px;
    }
    .goal-heading {
      color: #475569;
      font-size: 13px;
      font-weight: 800;
      line-height: 18px;
      margin-bottom: 4px;
    }
    .goal-value {
      line-height: 1.5;
      overflow-wrap: anywhere;
      white-space: pre-wrap;
    }
    .validation-list {
      display: grid;
      gap: 7px;
      list-style: none;
      margin: 6px 0 0;
      padding: 0;
    }
    .validation-list li {
      display: grid;
      grid-template-columns: 22px minmax(0, 1fr);
      column-gap: 9px;
      line-height: 1.5;
    }
    .validation-index {
      align-items: center;
      background: #ede9fe;
      border: 1px solid #c4b5fd;
      border-radius: 999px;
      color: #5b21b6;
      display: inline-flex;
      font-size: 11px;
      font-weight: 800;
      height: 20px;
      justify-content: center;
      line-height: 20px;
      margin-top: 1px;
      width: 20px;
    }
    .code-block {
      white-space: pre-wrap;
      overflow-wrap: anywhere;
      border: 1px solid #e2e8f0;
      border-radius: 6px;
      background: #f8fafc;
      padding: 16px;
      margin: 0;
      font: 13px/1.45 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    }
  </style>
</head>
<body>
  <h1>${escapeHTML(title)}</h1>
  <main class="detail-list">${detailsTextToHTML(detailText)}</main>
</body>
</html>`);
  popup.document.close();
}

function escapeHTML(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
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
