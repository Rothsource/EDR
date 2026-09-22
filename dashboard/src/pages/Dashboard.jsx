import React, { useState, useEffect } from 'react';
import { PageHeader } from '../components/PageHeader';
import { 
  Monitor, 
  Activity, 
  ShieldAlert, 
  Database, 
  RefreshCw, 
  ArrowUpRight, 
  Terminal, 
  X, 
  Copy, 
  Check 
} from 'lucide-react';
import { api } from '../api';
import { getSeverityBadge } from '../lib/severity';

// Heartbeat window rule: 90 seconds
const ONLINE_WINDOW_MS = 90 * 1000;

function isAgentOnline(agent) {
  if (agent.status === "revoked") return false;
  if (!agent.last_seen_at) return false;
  const lastSeen = new Date(agent.last_seen_at).getTime();
  return Date.now() - lastSeen < ONLINE_WINDOW_MS;
}

function relativeTime(isoString) {
  if (!isoString) return 'Never';
  const then = new Date(isoString).getTime();
  const diffSec = Math.floor((Date.now() - then) / 1000);
  if (diffSec < 5) return 'Just now';
  if (diffSec < 60) return `${diffSec}s ago`;
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return `${diffHr}h ago`;
  return `${Math.floor(diffHr / 24)}d ago`;
}

export const Dashboard = () => {
  const [agents, setAgents] = useState([]);
  const [recentEvents, setRecentEvents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [selectedEvent, setSelectedEvent] = useState(null);
  const [copied, setCopied] = useState(false);

  const loadData = async () => {
    setLoading(true);
    try {
      const [agentRes, eventRes] = await Promise.allSettled([
        api.listAgents(),
        api.listEvents({ limit: 15 })
      ]);

      if (agentRes.status === 'fulfilled' && Array.isArray(agentRes.value)) {
        setAgents(agentRes.value);
      }
      if (eventRes.status === 'fulfilled') {
        const val = eventRes.value;
        const items = val?.items ? val.items : (Array.isArray(val) ? val : []);
        setRecentEvents(items);
      }
    } catch (err) {
      console.error("Failed to load dashboard data:", err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 10000);
    return () => clearInterval(interval);
  }, []);

  const totalAgents = agents.length;
  // Checks actual heartbeat recency (last 90s) rather than static DB status
  const activeAgents = agents.filter((a) => isAgentOnline(a)).length;

  const handleCopyJson = () => {
    if (!selectedEvent) return;
    navigator.clipboard.writeText(JSON.stringify(selectedEvent, null, 2));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div>
      <PageHeader
        title="Fleet Security Overview"
        description="Unified EDR health metrics, real-time agent statuses, and raw telemetry ingestion."
        action={
          <button
            onClick={loadData}
            aria-label="Refresh dashboard data"
            className="p-2.5 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-xl transition-colors focus:ring-2 focus:ring-indigo-500"
            title="Refresh"
          >
            <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
          </button>
        }
      />

      {/* Metrics Row */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-5 mb-8">
        <div className="bg-slate-900/80 border border-slate-800 rounded-2xl p-6">
          <div className="flex items-center justify-between mb-4">
            <span className="text-sm font-medium text-slate-300">Total Endpoints</span>
            <div className="p-3 rounded-xl border bg-indigo-500/10 border-indigo-500/20 text-indigo-400">
              <Monitor className="h-5 w-5" />
            </div>
          </div>
          <div className="flex items-baseline justify-between">
            <h3 className="text-2xl font-bold text-slate-100">{activeAgents} / {totalAgents}</h3>
            <span className={`text-xs font-medium ${activeAgents > 0 ? 'text-emerald-400' : 'text-amber-400'}`}>
              {totalAgents > 0 ? `${activeAgents} Online` : '0 Nodes'}
            </span>
          </div>
        </div>

        <div className="bg-slate-900/80 border border-slate-800 rounded-2xl p-6">
          <div className="flex items-center justify-between mb-4">
            <span className="text-sm font-medium text-slate-300">Ingested Telemetry</span>
            <div className="p-3 rounded-xl border bg-emerald-500/10 border-emerald-500/20 text-emerald-400">
              <Activity className="h-5 w-5" />
            </div>
          </div>
          <div className="flex items-baseline justify-between">
            <h3 className="text-2xl font-bold text-slate-100">{recentEvents.length}</h3>
            <span className="text-xs font-medium text-emerald-400">Latest Batch</span>
          </div>
        </div>

        <div className="bg-slate-900/80 border border-slate-800 rounded-2xl p-6">
          <div className="flex items-center justify-between mb-4">
            <span className="text-sm font-medium text-slate-300">Fleet Security State</span>
            <div className="p-3 rounded-xl border bg-blue-500/10 border-blue-500/20 text-blue-400">
              <ShieldAlert className="h-5 w-5" />
            </div>
          </div>
          <div className="flex items-baseline justify-between">
            <h3 className="text-2xl font-bold text-slate-100">{activeAgents > 0 ? 'Protected' : 'Standby'}</h3>
            <span className="text-xs text-slate-400 font-medium">Zero Trust Active</span>
          </div>
        </div>

        <div className="bg-slate-900/80 border border-slate-800 rounded-2xl p-6">
          <div className="flex items-center justify-between mb-4">
            <span className="text-sm font-medium text-slate-300">Persistence Store</span>
            <div className="p-3 rounded-xl border bg-amber-500/10 border-amber-500/20 text-amber-400">
              <Database className="h-5 w-5" />
            </div>
          </div>
          <div className="flex items-baseline justify-between">
            <h3 className="text-xl font-bold text-slate-100">PostgreSQL</h3>
            <span className="text-xs text-emerald-400 font-mono">TIMESTAMPTZ</span>
          </div>
        </div>
      </div>

      {/* Live Stream Table */}
      <div className="bg-slate-900/80 border border-slate-800 rounded-2xl overflow-hidden backdrop-blur-sm">
        <div className="px-6 py-4 border-b border-slate-800 flex items-center justify-between">
          <div>
            <h3 className="text-base font-semibold text-slate-100">Live Ingestion Feed</h3>
            <p className="text-xs text-slate-400">Click any row to inspect deep OCSF attributes and payload</p>
          </div>
          <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-indigo-500/10 border border-indigo-500/20 text-indigo-400 text-xs font-mono">
            WebSocket Pipeline
          </span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr className="border-b border-slate-800 bg-slate-950/40 text-xs font-medium text-slate-400 uppercase tracking-wider">
                <th className="py-3.5 px-6">Event Time</th>
                <th className="py-3.5 px-6">Host / User</th>
                <th className="py-3.5 px-6">Severity</th>
                <th className="py-3.5 px-6">Type UID</th>
                <th className="py-3.5 px-6">Payload Preview</th>
                <th className="py-3.5 px-6 text-right">Inspect</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/80 text-sm text-slate-300">
              {recentEvents.length === 0 && !loading ? (
                <tr>
                  <td colSpan="6" className="py-8 text-center text-slate-400 text-sm">
                    No telemetry events received yet. Start your Go agent to begin streaming.
                  </td>
                </tr>
              ) : (
                recentEvents.map((ev) => (
                  <tr
                    key={ev.event_id}
                    onClick={() => setSelectedEvent(ev)}
                    className="hover:bg-slate-800/60 transition-colors cursor-pointer group"
                  >
                    <td className="py-4 px-6 font-mono text-xs text-slate-300">
                      {ev.time ? new Date(ev.time).toLocaleTimeString() : '—'}
                      <span className="block text-[11px] text-slate-400 font-sans">{relativeTime(ev.time)}</span>
                    </td>
                    <td className="py-4 px-6">
                      <span className="font-semibold text-slate-200 group-hover:text-indigo-400 transition-colors">
                        {ev.hostname || 'unknown'}
                      </span>
                      <span className="block text-[10px] text-slate-400 uppercase font-mono">
                        {ev.host_os || 'OS'} {ev.username ? `• ${ev.username}` : ''}
                      </span>
                    </td>
                    <td className="py-4 px-6">
                      {getSeverityBadge(ev.severity_id)}
                    </td>
                    <td className="py-4 px-6">
                      <span className="font-mono text-xs text-indigo-300 bg-indigo-500/10 border border-indigo-500/20 px-2 py-0.5 rounded">
                        {ev.type_uid}
                      </span>
                    </td>
                    <td className="py-4 px-6 font-mono text-xs text-slate-300 max-w-sm truncate">
                      {typeof ev.data === 'object' ? JSON.stringify(ev.data) : ev.data}
                    </td>
                    <td className="py-4 px-6 text-right">
                      <span className="text-xs text-slate-400 group-hover:text-slate-200 inline-flex items-center gap-1">
                        Details <ArrowUpRight className="h-3 w-3" />
                      </span>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Slide-out Inspector */}
      {selectedEvent && (
        <div className="fixed inset-0 z-50 flex justify-end bg-slate-950/70 backdrop-blur-sm">
          <div className="w-full max-w-2xl bg-slate-900 border-l border-slate-800 h-full overflow-y-auto shadow-2xl p-6 flex flex-col justify-between">
            <div>
              <div className="flex items-center justify-between pb-4 border-b border-slate-800">
                <div className="flex items-center gap-3">
                  <div className="p-2.5 rounded-xl bg-indigo-500/10 border border-indigo-500/20 text-indigo-400">
                    <Terminal className="h-5 w-5" />
                  </div>
                  <div>
                    <h3 className="text-base font-semibold text-slate-100">Telemetry Event Details</h3>
                    <span className="text-xs font-mono text-slate-400 truncate max-w-xs block">
                      {selectedEvent.event_id}
                    </span>
                  </div>
                </div>
                <button
                  onClick={() => setSelectedEvent(null)}
                  aria-label="Close details"
                  className="p-1.5 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800"
                >
                  <X className="h-5 w-5" />
                </button>
              </div>

              <div className="grid grid-cols-2 gap-4 my-6">
                <div className="bg-slate-950/60 border border-slate-800/80 p-3.5 rounded-xl">
                  <span className="text-[11px] font-medium text-slate-400 uppercase tracking-wider block mb-1">Time (UTC)</span>
                  <span className="text-xs font-mono text-slate-200">
                    {new Date(selectedEvent.time).toUTCString()}
                  </span>
                </div>

                <div className="bg-slate-950/60 border border-slate-800/80 p-3.5 rounded-xl">
                  <span className="text-[11px] font-medium text-slate-400 uppercase tracking-wider block mb-1">Severity & Class</span>
                  <div className="flex items-center gap-2">
                    {getSeverityBadge(selectedEvent.severity_id)}
                    <span className="text-xs font-mono text-slate-300">Class: {selectedEvent.class_uid}</span>
                  </div>
                </div>

                <div className="bg-slate-950/60 border border-slate-800/80 p-3.5 rounded-xl">
                  <span className="text-[11px] font-medium text-slate-400 uppercase tracking-wider block mb-1">Host & System</span>
                  <span className="text-xs font-semibold text-slate-200 block">
                    {selectedEvent.hostname} ({selectedEvent.host_os || 'OS'})
                  </span>
                  <span className="text-[10px] text-slate-400 font-mono">User: {selectedEvent.username || 'SYSTEM'}</span>
                </div>

                <div className="bg-slate-950/60 border border-slate-800/80 p-3.5 rounded-xl">
                  <span className="text-[11px] font-medium text-slate-400 uppercase tracking-wider block mb-1">Ingest Source</span>
                  <span className="text-xs font-mono text-emerald-400 uppercase">
                    {selectedEvent.ingest_source || 'websocket'}
                  </span>
                  <span className="text-[10px] text-slate-400 block font-mono">Agent ID: {selectedEvent.agent_id}</span>
                </div>
              </div>

              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-medium text-slate-300 uppercase tracking-wider">
                    OCSF Data Fields (`data` JSONB)
                  </span>
                  <button
                    onClick={handleCopyJson}
                    aria-label="Copy JSON payload"
                    className="flex items-center gap-1.5 px-3 py-1 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-lg text-xs transition-colors"
                  >
                    {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
                    {copied ? 'Copied' : 'Copy JSON'}
                  </button>
                </div>

                <pre className="bg-slate-950 border border-slate-800 p-4 rounded-xl text-xs font-mono text-indigo-300 overflow-x-auto whitespace-pre-wrap max-h-[380px]">
                  {JSON.stringify(selectedEvent.data, null, 2)}
                </pre>
              </div>
            </div>

            <div className="pt-4 border-t border-slate-800 flex justify-end">
              <button
                onClick={() => setSelectedEvent(null)}
                className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-xl text-xs font-medium"
              >
                Close Inspector
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default Dashboard;