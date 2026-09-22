import React, { useState, useEffect, useMemo } from "react";
import { PageHeader } from "../components/PageHeader";
import { 
  AlertTriangle, 
  ShieldAlert, 
  Clock, 
  Server, 
  ChevronDown, 
  ChevronUp, 
  RefreshCw, 
  CheckCircle2, 
  Eye, 
  ExternalLink 
} from "lucide-react";
import { api } from "../api";
import { getSeverityBadge } from "../lib/severity";

// Correlation threshold: 3+ failures within a rolling window of 5 minutes
const CORRELATION_WINDOW_MS = 5 * 60 * 1000;

export const Alerts = () => {
  const [events, setEvents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [expandedAlertId, setExpandedAlertId] = useState(null);
  const [alertStatuses, setAlertStatuses] = useState({}); // Local state: alertId -> 'New' | 'Investigating' | 'Resolved'

  const loadData = async () => {
    setLoading(true);
    try {
      const res = await api.listEvents({ limit: 150 });
      const items = res?.items ? res.items : (Array.isArray(res) ? res : []);
      setEvents(items);
    } catch (err) {
      console.error("Failed to load telemetry for alert correlation:", err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  // Client-side incident correlation engine
  const correlatedAlerts = useMemo(() => {
    // Only correlate failed auth (class_uid 3002)
    const authFailures = events.filter((e) => e.class_uid === 3002);

    // Group by source (src_ip if available, else hostname)
    const groups = {};
    authFailures.forEach((ev) => {
      const key = ev.data?.src_ip ? `IP:${ev.data.src_ip}` : `HOST:${ev.hostname}`;
      if (!groups[key]) groups[key] = [];
      groups[key].push(ev);
    });

    const incidents = [];

    Object.entries(groups).forEach(([sourceKey, groupEvents]) => {
      // Sort chronologically
      groupEvents.sort((a, b) => new Date(a.time).getTime() - new Date(b.time).getTime());

      // Rolling window aggregation
      let windowStart = 0;
      for (let i = 0; i < groupEvents.length; i++) {
        const currentTime = new Date(groupEvents[i].time).getTime();
        const startTime = new Date(groupEvents[windowStart].time).getTime();

        if (currentTime - startTime > CORRELATION_WINDOW_MS) {
          windowStart = i;
        }

        const cluster = groupEvents.slice(windowStart, i + 1);
        if (cluster.length >= 3 && (i === groupEvents.length - 1 || new Date(groupEvents[i + 1].time).getTime() - currentTime > CORRELATION_WINDOW_MS)) {
          const targetUsers = Array.from(new Set(cluster.map((e) => e.username || "unknown")));
          const count = cluster.length;

          // Severity logic derived from event frequency
          let severityId = 3; // Medium (3-4 failures)
          if (count >= 10) severityId = 5; // Critical
          else if (count >= 5) severityId = 4; // High

          const alertId = `inc-${sourceKey}-${startTime}`;
          incidents.push({
            id: alertId,
            sourceKey,
            sourceType: sourceKey.startsWith("IP:") ? "Remote IP" : "Local Host",
            sourceValue: sourceKey.replace(/^(IP:|HOST:)/, ""),
            eventCount: count,
            severityId,
            firstSeen: cluster[0].time,
            lastSeen: cluster[cluster.length - 1].time,
            targetUsers,
            service: cluster[0].data?.service || "sshd",
            underlyingEvents: cluster,
          });
        }
      }
    });

    return incidents.reverse();
  }, [events]);

  const cycleStatus = (alertId) => {
    setAlertStatuses((prev) => {
      const current = prev[alertId] || "New";
      const next = current === "New" ? "Investigating" : current === "Investigating" ? "Resolved" : "New";
      return { ...prev, [alertId]: next };
    });
  };

  return (
    <div>
      <PageHeader
        title="Correlated Incidents & Alerts"
        description="Automated multi-event correlation engine tracking brute-force attempts and privilege escalations."
        action={
          <button
            onClick={loadData}
            aria-label="Refresh incidents correlation"
            className="p-2.5 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-xl transition-colors focus:ring-2 focus:ring-indigo-500"
            title="Refresh Correlations"
          >
            <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
          </button>
        }
      />

      {correlatedAlerts.length === 0 && !loading ? (
        <div className="bg-slate-900/80 border border-slate-800 rounded-2xl p-12 text-center backdrop-blur-sm">
          <div className="h-12 w-12 rounded-2xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 flex items-center justify-center mx-auto mb-4">
            <CheckCircle2 className="h-6 w-6" />
          </div>
          <h3 className="text-base font-semibold text-slate-100">No Correlated Incidents — Telemetry is Quiet</h3>
          <p className="text-xs text-slate-400 max-w-md mx-auto mt-1">
            Authentication failure thresholds have not been breached within any 5-minute rolling window.
          </p>
        </div>
      ) : (
        <div className="space-y-4">
          {correlatedAlerts.map((alert) => {
            const isExpanded = expandedAlertId === alert.id;
            const status = alertStatuses[alert.id] || "New";

            const statusColors = {
              New: "bg-rose-500/10 text-rose-400 border-rose-500/20",
              Investigating: "bg-amber-500/10 text-amber-400 border-amber-500/20",
              Resolved: "bg-emerald-500/10 text-emerald-400 border-emerald-500/20",
            };

            return (
              <div
                key={alert.id}
                className="bg-slate-900/80 border border-slate-800 rounded-2xl overflow-hidden backdrop-blur-sm transition-all"
              >
                {/* Incident Card Header */}
                <div className="p-6 flex flex-col lg:flex-row lg:items-center justify-between gap-4">
                  <div className="flex items-start gap-4">
                    <div className="p-3 rounded-xl bg-slate-950 border border-slate-800 mt-1">
                      <ShieldAlert className="h-6 w-6 text-rose-400" />
                    </div>
                    <div>
                      <div className="flex items-center gap-2.5 flex-wrap mb-1">
                        <h3 className="text-base font-bold text-slate-100">
                          Brute-Force Anomaly: {alert.sourceValue}
                        </h3>
                        {getSeverityBadge(alert.severityId)}
                        <button
                          onClick={() => cycleStatus(alert.id)}
                          className={`px-2.5 py-0.5 rounded-full border text-xs font-medium cursor-pointer transition-colors ${statusColors[status]}`}
                          title="Click to cycle status"
                        >
                          Status: {status}
                        </button>
                      </div>
                      <p className="text-xs text-slate-400">
                        {alert.eventCount} failed {alert.service} authentication attempts targeted users:{" "}
                        <span className="text-indigo-300 font-mono font-medium">
                          {alert.targetUsers.join(", ")}
                        </span>
                      </p>
                    </div>
                  </div>

                  <div className="flex items-center gap-4 self-end lg:self-center">
                    <div className="text-right text-xs">
                      <span className="text-slate-500 block">First seen: {new Date(alert.firstSeen).toLocaleTimeString()}</span>
                      <span className="text-slate-300 font-medium block">Last seen: {new Date(alert.lastSeen).toLocaleTimeString()}</span>
                    </div>

                    <button
                      onClick={() => setExpandedAlertId(isExpanded ? null : alert.id)}
                      className="flex items-center gap-1.5 px-3 py-2 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-xl text-xs font-medium transition-colors"
                      aria-label="Toggle underlying events list"
                    >
                      {isExpanded ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
                      <span>{isExpanded ? "Collapse" : "Investigate"}</span>
                    </button>
                  </div>
                </div>

                {/* Expanded Raw Telemetry Breakdown */}
                {isExpanded && (
                  <div className="border-t border-slate-800/80 bg-slate-950/40 p-6">
                    <h4 className="text-xs font-semibold text-slate-300 uppercase tracking-wider mb-3">
                      Underlying Ingested Events ({alert.underlyingEvents.length})
                    </h4>
                    <div className="overflow-x-auto">
                      <table className="w-full text-left text-xs text-slate-300">
                        <thead>
                          <tr className="border-b border-slate-800 text-slate-400 font-mono">
                            <th className="py-2 px-3">Time</th>
                            <th className="py-2 px-3">Host</th>
                            <th className="py-2 px-3">User</th>
                            <th className="py-2 px-3">Reason</th>
                            <th className="py-2 px-3">Source IP</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-slate-800/40 font-mono">
                          {alert.underlyingEvents.map((ev) => (
                            <tr key={ev.event_id} className="hover:bg-slate-900/50">
                              <td className="py-2 px-3 text-slate-400">{new Date(ev.time).toLocaleTimeString()}</td>
                              <td className="py-2 px-3 text-slate-200">{ev.hostname}</td>
                              <td className="py-2 px-3 text-indigo-300">{ev.username || "—"}</td>
                              <td className="py-2 px-3 text-amber-400">{ev.data?.reason || "bad_password"}</td>
                              <td className="py-2 px-3 text-slate-400">{ev.data?.src_ip || "local"}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};

export default Alerts;