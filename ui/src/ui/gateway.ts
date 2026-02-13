// WebSocket Gateway Client for GoClaw

export type GatewayEventFrame = {
  type: "event";
  event: string;
  payload?: unknown;
  seq?: number;
};

export type GatewayResponseFrame = {
  type: "res";
  id: string;
  ok: boolean;
  payload?: unknown;
  error?: { code: string; message: string; details?: unknown };
};

export type GatewayHelloOk = {
  type: "hello-ok";
  protocol: number;
  sessionId?: string;
};

type Pending = {
  resolve: (value: unknown) => void;
  reject: (err: unknown) => void;
};

export type GatewayBrowserClientOptions = {
  url: string;
  token?: string;
  onHello?: (hello: GatewayHelloOk) => void;
  onEvent?: (evt: GatewayEventFrame) => void;
  onClose?: (info: { code: number; reason: string }) => void;
};

export class GatewayBrowserClient {
  private ws: WebSocket | null = null;
  private pending = new Map<string, Pending>();
  private closed = false;
  private lastSeq: number | null = null;
  private backoffMs = 800;
  private requestId = 0;

  constructor(private opts: GatewayBrowserClientOptions) {}

  start() {
    this.closed = false;
    this.connect();
  }

  stop() {
    this.closed = true;
    this.ws?.close();
    this.ws = null;
    this.flushPending(new Error("gateway client stopped"));
  }

  get connected() {
    return this.ws?.readyState === WebSocket.OPEN;
  }

  private connect() {
    if (this.closed) {
      return;
    }

    const url = this.opts.token
      ? `${this.opts.url}?token=${encodeURIComponent(this.opts.token)}`
      : this.opts.url;

    this.ws = new WebSocket(url);

    this.ws.addEventListener("open", () => {
      console.log("Gateway connected");
      this.backoffMs = 800;
    });

    this.ws.addEventListener("message", (ev) => {
      this.handleMessage(String(ev.data ?? ""));
    });

    this.ws.addEventListener("close", (ev) => {
      const reason = String(ev.reason ?? "");
      console.log(`Gateway closed (${ev.code}): ${reason}`);
      this.ws = null;
      this.flushPending(new Error(`gateway closed (${ev.code}): ${reason}`));
      this.opts.onClose?.({ code: ev.code, reason });
      this.scheduleReconnect();
    });

    this.ws.addEventListener("error", () => {
      console.error("Gateway error");
    });
  }

  private scheduleReconnect() {
    if (this.closed) {
      return;
    }
    const delay = this.backoffMs;
    this.backoffMs = Math.min(this.backoffMs * 1.7, 15_000);
    window.setTimeout(() => this.connect(), delay);
  }

  private flushPending(err: Error) {
    for (const [, p] of this.pending) {
      p.reject(err);
    }
    this.pending.clear();
  }

  private handleMessage(data: string) {
    try {
      const msg = JSON.parse(data);
      console.log("Received message:", msg);

      // Handle different message types
      if (msg.jsonrpc === "2.0") {
        // JSON-RPC response
        if (msg.method === "connected") {
          // Welcome message
          const hello: GatewayHelloOk = {
            type: "hello-ok",
            protocol: 2,
            sessionId: msg.params?.session_id,
          };
          this.opts.onHello?.(hello);
        } else if (msg.method) {
          // Notification/Event
          const evt: GatewayEventFrame = {
            type: "event",
            event: msg.method,
            payload: msg.params,
          };
          this.opts.onEvent?.(evt);
        } else if (msg.id) {
          // Response to our request
          const pending = this.pending.get(String(msg.id));
          if (pending) {
            this.pending.delete(String(msg.id));
            if (msg.error) {
              pending.reject(new Error(msg.error.message || "Request failed"));
            } else {
              pending.resolve(msg.result);
            }
          }
        }
      }
    } catch (err) {
      console.error("Failed to parse gateway message:", err);
    }
  }

  async call(method: string, params?: unknown): Promise<unknown> {
    if (!this.connected) {
      throw new Error("Gateway not connected");
    }

    const id = String(++this.requestId);
    const request = {
      jsonrpc: "2.0",
      id,
      method,
      params: params || {},
    };

    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });

      try {
        this.ws!.send(JSON.stringify(request));
      } catch (err) {
        this.pending.delete(id);
        reject(err);
      }

      // Timeout after 30 seconds
      setTimeout(() => {
        if (this.pending.has(id)) {
          this.pending.delete(id);
          reject(new Error("Request timeout"));
        }
      }, 30000);
    });
  }
}
