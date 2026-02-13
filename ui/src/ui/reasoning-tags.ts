/**
 * Strip <think></think>, <thinking></thinking>, <final></final> segments.
 * mode "preserve": keep content outside tags; trim "start" removes leading whitespace after stripping.
 */
export function stripReasoningTagsFromText(
  value: string,
  opts?: { mode?: string; trim?: string },
): string {
  let out = value
    .replace(/<think>[\s\S]*?<\/think>/gi, "\n")
    .replace(/<thinking>[\s\S]*?<\/thinking>/gi, "\n")
    .replace(/<final>([\s\S]*?)<\/final>/gi, "$1");
  if (opts?.trim === "start") {
    out = out.replace(/^\s+/, "");
  }
  return out.trim();
}
