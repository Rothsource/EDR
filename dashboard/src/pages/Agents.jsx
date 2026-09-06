import { useEffect, useState, useMemo } from "react";
import { api, AuthError } from "../api";
import { useAuth } from "../context/AuthContext";
import AppShell from "../components/AppShell";
import PageHeader from "../components/PageHeader";
import StatusBadge from "../components/StatusBadge";
import GenerateTokenModal from "../components/GenerateTokenModal";
import { getAgentState, relativeTime, formatDate } from "../agentStatus";

export default function Agents() {
  const { forceLogout } = useAuth();
  const [agents, setAgents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");
  const [showModal, setShowModal] = useState(false);
  const [actionError, setActionError] = useState("");
  const [confirmingDelete, setConfirmingDelete] = useState(null); // agent_id or null
  const [busyAgentId, setBusyAgentId] = useState(null); // disable buttons mid-request

  async function loadAgents() {
    setLoading(true);
    try {
      const data = await api.listAgents();
      setAgents(data);
      setError("");
    } catch (err) {
      if (err instanceof AuthError) {
        forceLogout();
        return;
      }
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    loadAgents();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const filtered = useMemo(() => {
    if (!search.trim()) return agents;
    const q = search.toLowerCase();
    return agents.filter((a) => a.hostname.toLowerCase().includes(q));
  }, [agents, search]);

  async function handleRevoke(agentId) {
    setActionError("");
    setBusyAgentId(agentId);
    try {
      await api.revokeAgent(agentId);
      await loadAgents();
    } catch (err) {
      if (err instanceof AuthError) {
        forceLogout();
        return;
      }
      setActionError(err.message);
    } finally {
      setBusyAgentId(null);
    }
  }

  async function handleDelete(agentId) {
    setActionError("");
    setBusyAgentId(agentId);
    try {
      await api.deleteAgent(agentId);
      await loadAgents();
    } catch (err) {
      if (err instanceof AuthError) {
        forceLogout();
        return;
      }
      setActionError(err.message);
    } finally {
      setBusyAgentId(null);
      setConfirmingDelete(null);
    }
  }

  async function handleUnrevoke(agentId) {
    setActionError("");
    setBusyAgentId(agentId);
    try {
      await api.unrevokeAgent(agentId);
      await loadAgents();
    } catch (err) {
      if (err instanceof AuthError) {
        forceLogout();
        return;
      }
      setActionError(err.message);
    } finally {
      setBusyAgentId(null);
    }
  }

  return (
    <AppShell>
      <PageHeader
        title="Agents"
        subtitle="All enrolled endpoints"
        actions={
          <button
            onClick={() => setShowModal(true)}
            className="h-9 px-4 rounded-lg bg-primary text-on-primary text-sm font-medium flex items-center gap-1.5 hover:opacity-95 transition"
          >
            <span className="material-symbols-outlined text-[18px]">vpn_key</span>
            Generate Enrollment Token
          </button>
        }
      />

      {error && (
        <div className="mb-4 p-3 rounded-lg bg-error-container text-on-error-container text-sm">
          {error}
        </div>
      )}

      {actionError && (
        <div className="mb-4 p-3 rounded-lg bg-error-container text-on-error-container text-sm">
          {actionError}
        </div>
      )}

      <div className="mb-4">
        <div className="relative w-64">
          <span className="material-symbols-outlined absolute left-3 top-1/2 -translate-y-1/2 text-outline text-[18px]">
            search
          </span>
          <input
            type="text"
            placeholder="Search hostname…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full h-9 pl-9 pr-3 rounded-lg bg-surface-container-low text-sm outline-none focus:ring-2 focus:ring-primary/40 transition"
          />
        </div>
      </div>

      <div className="bg-surface rounded-xl shadow-sm border border-outline-variant overflow-hidden">
        {loading ? (
          <div className="p-5 flex flex-col gap-3">
            {[...Array(4)].map((_, i) => (
              <div key={i} className="h-10 rounded-lg bg-surface-container animate-pulse" />
            ))}
          </div>
        ) : agents.length === 0 ? (
          <div className="p-10 text-center">
            <p className="text-sm text-on-surface-variant mb-4">
              No agents enrolled yet — generate a token to add your first device.
            </p>
            <button
              onClick={() => setShowModal(true)}
              className="h-9 px-4 rounded-lg bg-primary text-on-primary text-sm font-medium inline-flex items-center gap-1.5 hover:opacity-95 transition"
            >
              <span className="material-symbols-outlined text-[18px]">vpn_key</span>
              Generate Enrollment Token
            </button>
          </div>
        ) : filtered.length === 0 ? (
          <div className="p-10 text-center text-sm text-on-surface-variant">
            No agents match "{search}".
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-xs uppercase tracking-wide text-outline bg-surface-container-low">
                  <th className="px-5 py-3 font-medium">Status</th>
                  <th className="px-5 py-3 font-medium">Hostname</th>
                  <th className="px-5 py-3 font-medium">OS</th>
                  <th className="px-5 py-3 font-medium">IP Address</th>
                  <th className="px-5 py-3 font-medium">MAC Address</th>
                  <th className="px-5 py-3 font-medium">Enrolled</th>
                  <th className="px-5 py-3 font-medium">Last Seen</th>
                  <th className="px-5 py-3 font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((agent) => {
                  const isBusy = busyAgentId === agent.agent_id;
                  const isRevoked = agent.status === "revoked";
                  const isConfirmingThis = confirmingDelete === agent.agent_id;

                  return (
                    <tr
                      key={agent.agent_id}
                      className="border-t border-outline-variant hover:bg-surface-container-low transition-colors"
                    >
                      <td className="px-5 py-3">
                        <StatusBadge state={getAgentState(agent)} />
                      </td>
                      <td className="px-5 py-3 font-medium text-on-surface whitespace-nowrap">
                        {agent.hostname}
                      </td>
                      <td className="px-5 py-3 text-on-surface-variant whitespace-nowrap">
                        {agent.os}
                      </td>
                      <td className="px-5 py-3 text-on-surface-variant font-mono text-xs whitespace-nowrap">
                        {agent.ip_address || "—"}
                      </td>
                      <td className="px-5 py-3 text-on-surface-variant font-mono text-xs whitespace-nowrap">
                        {agent.mac_address || "—"}
                      </td>
                      <td className="px-5 py-3 text-on-surface-variant whitespace-nowrap">
                        {formatDate(agent.created_at)}
                      </td>
                      <td className="px-5 py-3 text-on-surface-variant whitespace-nowrap">
                        {relativeTime(agent.last_seen_at)}
                      </td>
                      <td className="px-5 py-3 whitespace-nowrap">
                        {isConfirmingThis ? (
                          <div className="flex items-center gap-2">
                            <span className="text-xs text-error font-medium">Delete?</span>
                            <button
                              onClick={() => handleDelete(agent.agent_id)}
                              disabled={isBusy}
                              className="text-xs px-2 py-1 rounded bg-error text-on-error font-medium disabled:opacity-50"
                            >
                              Confirm
                            </button>
                            <button
                              onClick={() => setConfirmingDelete(null)}
                              disabled={isBusy}
                              className="text-xs px-2 py-1 rounded text-on-surface-variant hover:bg-surface-container disabled:opacity-50"
                            >
                              Cancel
                            </button>
                          </div>
                        ) : (
                          <div className="flex items-center gap-2">
                            {isRevoked ? (
                              <button
                                onClick={() => handleUnrevoke(agent.agent_id)}
                                disabled={isBusy}
                                className="text-xs px-2 py-1 rounded text-success hover:bg-success-container disabled:opacity-40 font-medium"
                              >
                                Reactivate
                              </button>
                            ) : (
                              <button
                                onClick={() => handleRevoke(agent.agent_id)}
                                disabled={isBusy}
                                className="text-xs px-2 py-1 rounded text-warning hover:bg-warning-container disabled:opacity-40 font-medium"
                              >
                                Revoke
                              </button>
                            )}
                            <button
                              onClick={() => setConfirmingDelete(agent.agent_id)}
                              disabled={isBusy}
                              className="text-xs px-2 py-1 rounded text-error hover:bg-error-container disabled:opacity-40 font-medium"
                            >
                              Delete
                            </button>
                          </div>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {showModal && (
        <GenerateTokenModal
          onClose={() => {
            setShowModal(false);
            loadAgents(); // refresh in case a new agent already registered
          }}
        />
      )}
    </AppShell>
  );
}