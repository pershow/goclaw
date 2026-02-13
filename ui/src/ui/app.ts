import { LitElement, html, css } from "lit";
import { customElement, state } from "lit/decorators.js";
import { GatewayBrowserClient } from "./gateway.ts";
import type { AppState, ChatMessage, SessionInfo, AgentInfo } from "./types.ts";

@customElement("goclaw-app")
export class GoClawApp extends LitElement {
  static styles = css`
    :host {
      display: block;
      height: 100vh;
      width: 100vw;
      overflow: hidden;
    }

    .app-container {
      display: flex;
      flex-direction: column;
      height: 100%;
      background: var(--color-bg);
      color: var(--color-fg);
    }

    .app-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: var(--spacing-md);
      border-bottom: 1px solid var(--color-border);
      background: var(--color-bg);
    }

    .app-title {
      font-size: 20px;
      font-weight: 600;
      margin: 0;
    }

    .app-nav {
      display: flex;
      gap: var(--spacing-sm);
    }

    .nav-button {
      padding: var(--spacing-sm) var(--spacing-md);
      border: none;
      background: transparent;
      cursor: pointer;
      border-radius: var(--border-radius);
      transition: background var(--transition-speed);
    }

    .nav-button:hover {
      background: rgba(0, 0, 0, 0.05);
    }

    .nav-button.active {
      background: var(--color-primary);
      color: white;
    }

    .app-main {
      flex: 1;
      display: flex;
      overflow: hidden;
    }

    .sidebar {
      width: 250px;
      border-right: 1px solid var(--color-border);
      overflow-y: auto;
      padding: var(--spacing-md);
    }

    .content {
      flex: 1;
      overflow-y: auto;
      padding: var(--spacing-md);
    }

    .status-indicator {
      display: inline-block;
      width: 8px;
      height: 8px;
      border-radius: 50%;
      margin-right: var(--spacing-sm);
    }

    .status-indicator.connected {
      background: var(--color-success);
    }

    .status-indicator.disconnected {
      background: var(--color-danger);
    }

    .loading {
      display: flex;
      align-items: center;
      justify-content: center;
      height: 100%;
      font-size: 18px;
      color: var(--color-secondary);
    }

    .error {
      padding: var(--spacing-md);
      background: var(--color-danger);
      color: white;
      border-radius: var(--border-radius);
      margin: var(--spacing-md);
    }

    .chat-container {
      display: flex;
      flex-direction: column;
      height: 100%;
    }

    .messages {
      flex: 1;
      overflow-y: auto;
      padding: var(--spacing-md);
    }

    .message {
      margin-bottom: var(--spacing-md);
      padding: var(--spacing-md);
      border-radius: var(--border-radius);
      background: rgba(0, 0, 0, 0.02);
    }

    .message.user {
      background: var(--color-primary);
      color: white;
      margin-left: 20%;
    }

    .message.assistant {
      background: rgba(0, 0, 0, 0.05);
      margin-right: 20%;
    }

    .message-role {
      font-weight: 600;
      margin-bottom: var(--spacing-sm);
      font-size: 12px;
      text-transform: uppercase;
      opacity: 0.7;
    }

    .message-content {
      white-space: pre-wrap;
      word-wrap: break-word;
    }

    .input-area {
      border-top: 1px solid var(--color-border);
      padding: var(--spacing-md);
      display: flex;
      gap: var(--spacing-sm);
    }

    .input-area textarea {
      flex: 1;
      min-height: 60px;
      resize: vertical;
    }

    .session-list {
      list-style: none;
      padding: 0;
      margin: 0;
    }

    .session-item {
      padding: var(--spacing-sm);
      cursor: pointer;
      border-radius: var(--border-radius);
      margin-bottom: var(--spacing-xs);
    }

    .session-item:hover {
      background: rgba(0, 0, 0, 0.05);
    }

    .session-item.active {
      background: var(--color-primary);
      color: white;
    }

    .empty-state {
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      height: 100%;
      color: var(--color-secondary);
      text-align: center;
      padding: var(--spacing-xl);
    }

    .empty-state h2 {
      margin-bottom: var(--spacing-md);
    }
  `;

  @state() private state: AppState = {
    connected: false,
    sessionId: null,
    currentView: "chat",
    theme: "system",
    channels: null,
    sessions: [],
    agents: [],
    messages: [],
    loading: true,
    error: null,
  };

  private gateway: GatewayBrowserClient | null = null;

  connectedCallback() {
    super.connectedCallback();
    this.initGateway();
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this.gateway?.stop();
  }

  private initGateway() {
    const wsUrl = `ws://${window.location.hostname}:28789/ws`;

    this.gateway = new GatewayBrowserClient({
      url: wsUrl,
      onHello: (hello) => {
        console.log("Gateway hello:", hello);
        this.state = {
          ...this.state,
          connected: true,
          loading: false,
          sessionId: hello.sessionId || null,
        };
        this.loadInitialData();
      },
      onEvent: (evt) => {
        console.log("Gateway event:", evt);
        this.handleGatewayEvent(evt);
      },
      onClose: (info) => {
        console.log("Gateway closed:", info);
        this.state = {
          ...this.state,
          connected: false,
        };
      },
    });

    this.gateway.start();
  }

  private async loadInitialData() {
    try {
      // Load sessions, agents, etc.
      // This would call gateway methods to fetch data
      console.log("Loading initial data...");
    } catch (err) {
      console.error("Failed to load initial data:", err);
      this.state = {
        ...this.state,
        error: String(err),
      };
    }
  }

  private handleGatewayEvent(evt: any) {
    // Handle different event types
    switch (evt.event) {
      case "message.outbound":
        // Handle outbound message
        break;
      case "channel.status":
        // Handle channel status update
        break;
      default:
        console.log("Unhandled event:", evt.event);
    }
  }

  private handleViewChange(view: string) {
    this.state = {
      ...this.state,
      currentView: view,
    };
  }

  private async handleSendMessage(content: string) {
    if (!this.gateway || !content.trim()) {
      return;
    }

    try {
      // Add user message to UI
      const userMessage: ChatMessage = {
        id: String(Date.now()),
        role: "user",
        content,
        timestamp: Date.now(),
      };

      this.state = {
        ...this.state,
        messages: [...this.state.messages, userMessage],
      };

      // Send to gateway
      await this.gateway.call("chat.send", {
        content,
        sessionId: this.state.sessionId,
      });
    } catch (err) {
      console.error("Failed to send message:", err);
      this.state = {
        ...this.state,
        error: String(err),
      };
    }
  }

  render() {
    if (this.state.loading) {
      return html`
        <div class="app-container">
          <div class="loading">Connecting to GoClaw Gateway...</div>
        </div>
      `;
    }

    return html`
      <div class="app-container">
        ${this.renderHeader()} ${this.renderMain()}
      </div>
    `;
  }

  private renderHeader() {
    return html`
      <div class="app-header">
        <h1 class="app-title">
          <span
            class="status-indicator ${this.state.connected
              ? "connected"
              : "disconnected"}"
          ></span>
          GoClaw Control UI
        </h1>
        <div class="app-nav">
          <button
            class="nav-button ${this.state.currentView === "chat"
              ? "active"
              : ""}"
            @click=${() => this.handleViewChange("chat")}
          >
            Chat
          </button>
          <button
            class="nav-button ${this.state.currentView === "channels"
              ? "active"
              : ""}"
            @click=${() => this.handleViewChange("channels")}
          >
            Channels
          </button>
          <button
            class="nav-button ${this.state.currentView === "sessions"
              ? "active"
              : ""}"
            @click=${() => this.handleViewChange("sessions")}
          >
            Sessions
          </button>
          <button
            class="nav-button ${this.state.currentView === "config"
              ? "active"
              : ""}"
            @click=${() => this.handleViewChange("config")}
          >
            Config
          </button>
        </div>
      </div>
    `;
  }

  private renderMain() {
    if (this.state.error) {
      return html`
        <div class="error">
          <strong>Error:</strong> ${this.state.error}
          <button @click=${() => (this.state = { ...this.state, error: null })}>
            Dismiss
          </button>
        </div>
      `;
    }

    return html`
      <div class="app-main">
        ${this.state.currentView === "chat" ? this.renderChatView() : ""}
        ${this.state.currentView === "channels"
          ? this.renderChannelsView()
          : ""}
        ${this.state.currentView === "sessions"
          ? this.renderSessionsView()
          : ""}
        ${this.state.currentView === "config" ? this.renderConfigView() : ""}
      </div>
    `;
  }

  private renderChatView() {
    return html`
      <div class="sidebar">
        <h3>Sessions</h3>
        ${this.state.sessions.length === 0
          ? html`<p>No sessions yet</p>`
          : html`
              <ul class="session-list">
                ${this.state.sessions.map(
                  (session) => html`
                    <li
                      class="session-item ${session.id ===
                      this.state.sessionId
                        ? "active"
                        : ""}"
                      @click=${() =>
                        (this.state = {
                          ...this.state,
                          sessionId: session.id,
                        })}
                    >
                      ${session.id.substring(0, 8)}...
                    </li>
                  `
                )}
              </ul>
            `}
      </div>
      <div class="content">${this.renderChat()}</div>
    `;
  }

  private renderChat() {
    return html`
      <div class="chat-container">
        <div class="messages">
          ${this.state.messages.length === 0
            ? html`
                <div class="empty-state">
                  <h2>Welcome to GoClaw</h2>
                  <p>Start a conversation by typing a message below.</p>
                </div>
              `
            : this.state.messages.map(
                (msg) => html`
                  <div class="message ${msg.role}">
                    <div class="message-role">${msg.role}</div>
                    <div class="message-content">${msg.content}</div>
                  </div>
                `
              )}
        </div>
        <div class="input-area">
          <textarea
            placeholder="Type your message..."
            @keydown=${(e: KeyboardEvent) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                const target = e.target as HTMLTextAreaElement;
                this.handleSendMessage(target.value);
                target.value = "";
              }
            }}
          ></textarea>
          <button
            @click=${(e: Event) => {
              const textarea = (e.target as HTMLElement)
                .previousElementSibling as HTMLTextAreaElement;
              this.handleSendMessage(textarea.value);
              textarea.value = "";
            }}
          >
            Send
          </button>
        </div>
      </div>
    `;
  }

  private renderChannelsView() {
    return html`
      <div class="content">
        <h2>Channels</h2>
        <p>Channel management coming soon...</p>
      </div>
    `;
  }

  private renderSessionsView() {
    return html`
      <div class="content">
        <h2>Sessions</h2>
        <p>Session management coming soon...</p>
      </div>
    `;
  }

  private renderConfigView() {
    return html`
      <div class="content">
        <h2>Configuration</h2>
        <p>Configuration management coming soon...</p>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    "goclaw-app": GoClawApp;
  }
}
