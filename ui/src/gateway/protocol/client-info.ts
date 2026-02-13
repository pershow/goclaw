export const GATEWAY_CLIENT_NAMES = {
  CONTROL_UI: "control-ui",
  TUI: "tui",
  WEBCHAT: "webchat",
} as const;

export const GATEWAY_CLIENT_MODES = {
  WEBCHAT: "webchat",
  TUI: "tui",
} as const;

export type GatewayClientName = (typeof GATEWAY_CLIENT_NAMES)[keyof typeof GATEWAY_CLIENT_NAMES];
export type GatewayClientMode = (typeof GATEWAY_CLIENT_MODES)[keyof typeof GATEWAY_CLIENT_MODES];
