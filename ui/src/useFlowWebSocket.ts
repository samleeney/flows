import { useEffect, useRef, useCallback } from "react";
import type { FlowJSON, WSMessage } from "./types";

export function useFlowWebSocket(
  onFlowUpdate: (flow: FlowJSON) => void
) {
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(`${protocol}//${window.location.host}/ws`);
    wsRef.current = ws;

    ws.onmessage = (event) => {
      const msg: WSMessage = JSON.parse(event.data);
      if (msg.type === "flow") {
        onFlowUpdate(msg.data as FlowJSON);
      }
    };

    ws.onclose = () => {
      console.log("WebSocket closed");
    };

    return () => {
      ws.close();
    };
  }, [onFlowUpdate]);

  const sendUpdate = useCallback((flow: FlowJSON) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(
        JSON.stringify({ type: "update_flow", data: flow })
      );
    }
  }, []);

  return { sendUpdate };
}
