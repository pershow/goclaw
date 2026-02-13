/**
 * Relative time: "Xs ago", "Xm ago", "in <1m", "5m from now", etc.
 */
export function formatRelativeTimestamp(ts: number | null | undefined): string {
  if (ts == null || !Number.isFinite(ts)) {
    return "n/a";
  }
  const now = Date.now();
  const deltaMs = ts - now;
  const deltaSec = Math.round(deltaMs / 1000);
  const deltaMin = Math.round(deltaMs / 60_000);
  const deltaHour = Math.round(deltaMs / 3_600_000);
  const deltaDay = Math.round(deltaMs / 86_400_000);

  if (deltaMs >= 0) {
    if (deltaSec < 60) return "in <1m";
    if (deltaMin < 60) return `${deltaMin}m from now`;
    if (deltaHour < 48) return `${deltaHour}h from now`;
    return `${deltaDay}d from now`;
  }
  const absSec = Math.abs(deltaSec);
  const absMin = Math.abs(deltaMin);
  const absHour = Math.abs(deltaHour);
  const absDay = Math.abs(deltaDay);
  if (absSec < 60) return `${absSec}s ago`;
  if (absMin < 60) return `${absMin}m ago`;
  if (absHour < 24) return `${absHour}h ago`;
  if (absDay < 7) return `${absDay}d ago`;
  return "long ago";
}

/**
 * Human-readable duration, e.g. "2m 30s", "1h"
 */
export function formatDurationHuman(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return "n/a";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  const sec = Math.floor(ms / 1000) % 60;
  const min = Math.floor(ms / 60_000) % 60;
  const hour = Math.floor(ms / 3_600_000);
  const parts: string[] = [];
  if (hour > 0) parts.push(`${hour}h`);
  if (min > 0) parts.push(`${min}m`);
  if (sec > 0 || parts.length === 0) parts.push(`${sec}s`);
  return parts.join(" ");
}

/**
 * Compact duration, e.g. "2m 30s" or "1h 0m"
 */
export function formatDurationCompact(
  ms?: number | null,
  opts?: { spaced?: boolean },
): string | null {
  if (ms == null || !Number.isFinite(ms) || ms < 0) return null;
  const sep = opts?.spaced ? " " : "";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  const sec = Math.floor(ms / 1000) % 60;
  const min = Math.floor(ms / 60_000) % 60;
  const hour = Math.floor(ms / 3_600_000);
  const parts: string[] = [];
  if (hour > 0) parts.push(`${hour}h`);
  if (min > 0) parts.push(`${min}m`);
  if (sec > 0 || parts.length === 0) parts.push(`${sec}s`);
  return parts.join(sep);
}
