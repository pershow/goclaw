/**
 * Normalize tool name for policy matching (trim, lowercase).
 */
export function normalizeToolName(name?: string): string {
  if (name == null) return "";
  return String(name).trim().toLowerCase();
}

/** Known group names that expand to tool ids (e.g. "exec" -> ["exec", "apply_patch"]). */
const TOOL_GROUP_EXPANSIONS: Record<string, string[]> = {
  exec: ["exec", "apply_patch"],
};

/**
 * Expand group names in a pattern list to concrete tool names.
 * Unknown entries are passed through as-is.
 */
export function expandToolGroups(patterns: string[]): string[] {
  const out: string[] = [];
  for (const p of patterns) {
    const n = normalizeToolName(p);
    const expanded = TOOL_GROUP_EXPANSIONS[n];
    if (expanded) {
      out.push(...expanded);
    } else {
      out.push(n || p.trim());
    }
  }
  return out;
}

export type ToolProfilePolicy = { allow: string[]; deny: string[] };

/**
 * Resolve a profile name (e.g. "full", "minimal") to allow/deny lists.
 * Returns null if profile is unknown.
 */
export function resolveToolProfilePolicy(profile: unknown): ToolProfilePolicy | null {
  const name = normalizeToolName(typeof profile === "string" ? profile : "");
  if (!name) return null;
  if (name === "full") {
    return { allow: ["*"], deny: [] };
  }
  if (name === "minimal" || name === "restricted") {
    return { allow: [], deny: [] };
  }
  return null;
}
