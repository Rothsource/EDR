import React from "react";

export const SEVERITY_CONFIG = {
  6: { label: "Critical", color: "rose", bg: "bg-rose-500/10", border: "border-rose-500/20", text: "text-rose-400", dot: "bg-rose-400" },
  5: { label: "Critical", color: "rose", bg: "bg-rose-500/10", border: "border-rose-500/20", text: "text-rose-400", dot: "bg-rose-400" },
  4: { label: "High", color: "orange", bg: "bg-orange-500/10", border: "border-orange-500/20", text: "text-orange-400", dot: "bg-orange-400" },
  3: { label: "Medium", color: "amber", bg: "bg-amber-500/10", border: "border-amber-500/20", text: "text-amber-400", dot: "bg-amber-400" },
  2: { label: "Low", color: "blue", bg: "bg-blue-500/10", border: "border-blue-500/20", text: "text-blue-400", dot: "bg-blue-400" },
  1: { label: "Info", color: "slate", bg: "bg-slate-800/80", border: "border-slate-700/60", text: "text-slate-300", dot: "bg-slate-400" },
  0: { label: "Info", color: "slate", bg: "bg-slate-800/80", border: "border-slate-700/60", text: "text-slate-300", dot: "bg-slate-400" },
};

export function getSeverityBadge(severityId) {
  const cfg = SEVERITY_CONFIG[severityId] || SEVERITY_CONFIG[0];
  return (
    <span
      className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md text-xs font-mono font-medium border ${cfg.bg} ${cfg.border} ${cfg.text}`}
    >
      <span className={`h-1.5 w-1.5 rounded-full ${cfg.dot}`} />
      {cfg.label}
    </span>
  );
}