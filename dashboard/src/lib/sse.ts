"use client";

import { useEffect, useRef, useState } from "react";
import { sseBase } from "./api-client";

type ConnectionState = "connecting" | "open" | "error" | "closed";

interface UseSSEResult<T> {
  data: T | null;
  state: ConnectionState;
  error: string | null;
}

export function useSSE<T>(path: string | null): UseSSEResult<T> {
  const [data, setData] = useState<T | null>(null);
  const [state, setState] = useState<ConnectionState>("connecting");
  const [error, setError] = useState<string | null>(null);
  const reconnectRef = useRef<number | null>(null);

  useEffect(() => {
    if (!path) {
      setState("closed");
      return;
    }

    let closed = false;
    let source: EventSource | null = null;

    const connect = () => {
      if (closed) return;
      setState("connecting");
      source = new EventSource(`${sseBase}${path}`);

      source.onopen = () => {
        setState("open");
        setError(null);
      };
      source.onmessage = (e) => {
        try {
          setData(JSON.parse(e.data) as T);
        } catch (err) {
          setError(err instanceof Error ? err.message : String(err));
        }
      };
      source.onerror = () => {
        setState("error");
        setError("connection lost");
        source?.close();
        if (!closed) {
          reconnectRef.current = window.setTimeout(connect, 2000);
        }
      };
    };

    connect();

    return () => {
      closed = true;
      if (reconnectRef.current !== null) {
        window.clearTimeout(reconnectRef.current);
        reconnectRef.current = null;
      }
      source?.close();
      setState("closed");
    };
  }, [path]);

  return { data, state, error };
}
