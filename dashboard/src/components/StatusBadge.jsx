// Every status is shown with BOTH a color and a text label — never color
// alone, for accessibility (and just generally clearer at a glance).
const STYLES = {
  online: { dot: "bg-success", bg: "bg-success-container", text: "text-success", label: "Online" },
  offline: { dot: "bg-outline", bg: "bg-surface-container", text: "text-outline", label: "Offline" },
  revoked: { dot: "bg-error", bg: "bg-error-container", text: "text-error", label: "Revoked" },
};

export default function StatusBadge({ state }) {
  const style = STYLES[state] || STYLES.offline;
  return (
    <span
      className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium ${style.bg} ${style.text}`}
    >
      <span className={`w-1.5 h-1.5 rounded-full ${style.dot}`} />
      {style.label}
    </span>
  );
}