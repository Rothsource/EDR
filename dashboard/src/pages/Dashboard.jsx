import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, AuthError } from "../api";
import { useAuth } from "../context/AuthContext";
import AppShell from "../components/AppShell";
import PageHeader from "../components/PageHeader";
import StatusBadge from "../components/StatusBadge";
import { isAgentOnline, getAgentState, relativeTime, formatDate } from "../agentStatus";

export default function Dashboard() {
  const { forceLogout } = useAuth();
  const [agents, setAgents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const data = await api.listAgents();
        if (!cancelled) setAgents(data);
      } catch (err) {
        if (err instanceof AuthError) {
          forceLogout();
          return;
        }
        if (!cancelled) setError(err.message);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    load();
    return () => {
      cancelled = true;
    };
  }, [forceLogout]);

  const total = agents.length;
  const online = agents.filter(isAgentOnline).length;
  const offline = total - online;
  const recent = [...agents]
    .sort((a, b) => new Date(b.created_at) - new Date(a.created_at))
    .slice(0, 5);

  return (
    <AppShell>
      <PageHeader
        title="Dashboard"
        subtitle="Overview of enrolled endpoints"
      />

      {error && (
        <div className="mb-6 p-3 rounded-lg bg-error-container text-on-error-container text-sm">
          {error}
        </div>
      )}

      <div className="grid grid-cols-3 gap-4 mb-6">
        <StatCard
          label="Total Agents"
          value={loading ? "—" : total}
          icon="computer"
          tone="neutral"
        />
        <StatCard
          label="Online Now"
          value={loading ? "—" : online}
          icon="check_circle"
          tone="success"
        />
        <StatCard
          label="Offline"
          value={loading ? "—" : offline}
          icon="cancel"
          tone={offline > 0 ? "error" : "neutral"}
        />
      </div>

      <div className="bg-surface rounded-xl shadow-sm border border-outline-variant">
        <div className="flex items-center justify-between px-5 py-4 border-b border-outline-variant">
          <h2 className="text-sm font-semibold text-on-surface">
            Recent Agents{" "}
            <span className="text-on-surface-variant font-normal">
              (showing {recent.length} of {total})
            </span>
          </h2>
          <Link to="/agents" className="text-sm text-primary font-medium hover:underline">
            View all →
          </Link>
        </div>

        {loading ? (
          <div className="p-5 text-sm text-on-surface-variant">Loading…</div>
        ) : recent.length === 0 ? (
          <EmptyAgentsRow />
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs uppercase tracking-wide text-outline">
                <th className="px-5 py-3 font-medium">Status</th>
                <th className="px-5 py-3 font-medium">Hostname</th>
                <th className="px-5 py-3 font-medium">OS</th>
                <th className="px-5 py-3 font-medium">Enrolled</th>
                <th className="px-5 py-3 font-medium">Last Seen</th>
              </tr>
            </thead>
            <tbody>
              {recent.map((agent) => (
                <tr key={agent.agent_id} className="border-t border-outline-variant">
                  <td className="px-5 py-3">
                    <StatusBadge state={getAgentState(agent)} />
                  </td>
                  <td className="px-5 py-3 font-medium text-on-surface">{agent.hostname}</td>
                  <td className="px-5 py-3 text-on-surface-variant">{agent.os}</td>
                  <td className="px-5 py-3 text-on-surface-variant">
                    {formatDate(agent.created_at)}
                  </td>
                  <td className="px-5 py-3 text-on-surface-variant">
                    {relativeTime(agent.last_seen_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </AppShell>
  );
}

function StatCard({ label, value, icon, tone }) {
  const toneClasses = {
    neutral: "bg-surface-container text-on-surface-variant",
    success: "bg-success-container text-success",
    error: "bg-error-container text-error",
  }[tone];

  return (
    <div className="bg-surface rounded-xl shadow-sm border border-outline-variant p-5 flex items-center justify-between">
      <div>
        <p className="text-xs uppercase tracking-wide text-outline mb-1">{label}</p>
        <p className="text-2xl font-semibold text-on-surface">{value}</p>
      </div>
      <div className={`w-10 h-10 rounded-lg flex items-center justify-center ${toneClasses}`}>
        <span className="material-symbols-outlined text-[20px]">{icon}</span>
      </div>
    </div>
  );
}

function EmptyAgentsRow() {
  return (
    <div className="p-8 text-center">
      <p className="text-sm text-on-surface-variant mb-3">No agents enrolled yet.</p>
      <Link
        to="/agents"
        className="inline-flex items-center gap-1.5 text-sm text-primary font-medium hover:underline"
      >
        Go generate an enrollment token
        <span className="material-symbols-outlined text-[16px]">arrow_forward</span>
      </Link>
    </div>
  );
}