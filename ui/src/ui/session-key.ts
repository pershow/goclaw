/**
 * Parse session key to extract agent id when key is in form "agentId/..." or "agentId".
 * For keys like "direct" or "main", returns undefined so callers can use default.
 */
export function parseAgentSessionKey(sessionKey: string): { agentId: string } | null {
  if (!sessionKey || typeof sessionKey !== "string") {
    return null;
  }
  const trimmed = sessionKey.trim();
  if (!trimmed) {
    return null;
  }
  const slash = trimmed.indexOf("/");
  if (slash >= 0) {
    const agentId = trimmed.slice(0, slash).trim();
    return agentId ? { agentId } : null;
  }
  if (trimmed === "direct" || trimmed === "main" || trimmed === "global") {
    return null;
  }
  return { agentId: trimmed };
}
