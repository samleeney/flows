import { useEffect, useRef, useCallback } from "react";
import type { FlowJSON, WSMessage, RunSnapshot, EventEnvelope } from "./types";

export interface LiveHandlers {
  onFlowUpdate: (flow: FlowJSON) => void;
  onRunSnapshot: (snap: RunSnapshot) => void;
  onRunEvent: (env: EventEnvelope) => void;
}

export function useFlowWebSocket(handlers: LiveHandlers) {
  const wsRef = useRef<WebSocket | null>(null);
  const handlersRef = useRef(handlers);
  handlersRef.current = handlers;

  useEffect(() => {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(`${protocol}//${window.location.host}/ws`);
    wsRef.current = ws;

    ws.onmessage = (event) => {
      let msg: WSMessage;
      try {
        msg = JSON.parse(event.data);
      } catch {
        return;
      }
      switch (msg.type) {
        case "flow":
          handlersRef.current.onFlowUpdate(msg.data as FlowJSON);
          break;
        case "run_snapshot":
          handlersRef.current.onRunSnapshot(msg.data as RunSnapshot);
          break;
        case "run_event":
          handlersRef.current.onRunEvent(msg.data as EventEnvelope);
          break;
      }
    };

    ws.onclose = () => {
      console.log("WebSocket closed");
    };

    return () => {
      ws.close();
    };
  }, []);

  const sendUpdate = useCallback((flow: FlowJSON) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(
        JSON.stringify({ type: "update_flow", data: flow })
      );
    }
  }, []);

  return { sendUpdate };
}
