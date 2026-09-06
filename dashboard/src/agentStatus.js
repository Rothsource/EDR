// Your backend's heartbeat threshold (routers/agent.py) is 60 seconds, but
// that's meant for the agent's own retry logic, not display. A short buffer
// avoids the UI flickering "offline" the instant one heartbeat is a few
// seconds late — 90s is a reasonable, still-responsive window for the UI.
const ONLINE_WINDOW_MS = 90 * 1000;

// Three states now that revoke exists: "revoked" (explicit admin action,
// takes priority over everything else), "online" (recent heartbeat), or
// "offline" (no recent heartbeat, but not revoked).
export function getAgentState(agent) {
  if (agent.status === "revoked") return "revoked";
  if (!agent.last_seen_at) return "offline";
  const lastSeen = new Date(agent.last_seen_at).getTime();
  return Date.now() - lastSeen < ONLINE_WINDOW_MS ? "online" : "offline";
}

// Kept for any callers that only care about the boolean online/offline split.
export function isAgentOnline(agent) {
  return getAgentState(agent) === "online";
}

export function relativeTime(isoString) {
  if (!isoString) return "Never";
  const then = new Date(isoString).getTime();
  const diffSec = Math.floor((Date.now() - then) / 1000);

  if (diffSec < 5) return "Just now";
  if (diffSec < 60) return `${diffSec}s ago`;
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return `${diffHr}h ago`;
  const diffDay = Math.floor(diffHr / 24);
  return `${diffDay}d ago`;
}

export function formatDate(isoString) {
  if (!isoString) return "—";
  return new Date(isoString).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}