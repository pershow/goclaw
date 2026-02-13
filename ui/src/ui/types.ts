// Core types for GoClaw Control UI

export type ChannelAccountSnapshot = {
  accountId: string;
  name?: string | null;
  enabled?: boolean | null;
  configured?: boolean | null;
  linked?: boolean | null;
  running?: boolean | null;
  connected?: boolean | null;
  reconnectAttempts?: number | null;
  lastConnectedAt?: number | null;
  lastError?: string | null;
  lastStartAt?: number | null;
  lastStopAt?: number | null;
  lastInboundAt?: number | null;
  lastOutboundAt?: number | null;
  mode?: string | null;
  webhookUrl?: string | null;
};

export type ChannelsStatusSnapshot = {
  ts: number;
  channelOrder: string[];
  channelLabels: Record<string, string>;
  channels: Record<string, unknown>;
  channelAccounts: Record<string, ChannelAccountSnapshot[]>;
  channelDefaultAccountId: Record<string, string>;
};

export type SessionInfo = {
  id: string;
  agentId?: string;
  createdAt: number;
  updatedAt: number;
  messageCount?: number;
};

export type AgentInfo = {
  id: string;
  name: string;
  model?: string;
  provider?: string;
  systemPrompt?: string;
};

export type ChatMessage = {
  id: string;
  role: "user" | "assistant" | "system";
  content: string;
  timestamp: number;
  toolCalls?: ToolCall[];
  toolResults?: ToolResult[];
};

export type ToolCall = {
  id: string;
  name: string;
  arguments: Record<string, unknown>;
};

export type ToolResult = {
  id: string;
  output: string;
  error?: string;
};

export type GatewayConfig = {
  host: string;
  port: number;
  enableAuth: boolean;
  websocket: {
    host: string;
    port: number;
    path: string;
  };
};

export type AppState = {
  connected: boolean;
  sessionId: string | null;
  currentView: string;
  theme: "light" | "dark" | "system";
  channels: ChannelsStatusSnapshot | null;
  sessions: SessionInfo[];
  agents: AgentInfo[];
  messages: ChatMessage[];
  loading: boolean;
  error: string | null;
};
