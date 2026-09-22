import React, { useState, useEffect } from "react";
import { PageHeader } from "../components/PageHeader";
import { GenerateTokenModal } from "../components/GenerateTokenModal";
import { Monitor, Plus, Trash2, ShieldAlert, ShieldCheck, RefreshCw } from "lucide-react";
import { api } from "../api";
import { getAgentState, relativeTime, formatDate } from "../agentStatus";

export const Agents = () => {
  const [isTokenModalOpen, setIsTokenModalOpen] = useState(false);
  const [agents, setAgents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const loadAgents = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.listAgents();
      setAgents(data || []);
    } catch (err) {
      setError(err.message || "Failed to load agents from server");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadAgents();
  }, []);

  const handleToggleRevoke = async (agent) => {
    try {
      if (agent.status === "revoked") {
        await api.unrevokeAgent(agent.agent_id);
      } else {
        await api.revokeAgent(agent.agent_id);
      }
      await loadAgents();
    } catch (err) {
      alert(`Action failed: ${err.message}`);
    }
  };

  const handleDelete = async (agentId) => {
    if (!confirm("Are you sure you want to permanently delete this agent?")) return;
    try {
      await api.deleteAgent(agentId);
      await loadAgents();
    } catch (err) {
      alert(`Delete failed: ${err.message}`);
    }
  };

  return (
    <div>
      <PageHeader
        title="Agent Fleet Management"
        description="Endpoints active and communicating with your backend."
        action={
          <div className="flex gap-2">
            <button
              onClick={loadAgents}
              className="p-2.5 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-xl transition-colors"
              title="Refresh Fleet"
            >
              <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
            </button>
            <button
              onClick={() => setIsTokenModalOpen(true)}
              className="flex items-center gap-2 px-4 py-2.5 bg-indigo-600 hover:bg-indigo-500 text-white rounded-xl text-sm font-medium transition-colors shadow-lg shadow-indigo-600/20"
            >
              <Plus className="h-4 w-4" />
              Generate Enrollment Token
            </button>
          </div>
        }
      />

      {error && (
        <div className="mb-6 p-4 bg-rose-500/10 border border-rose-500/20 rounded-xl text-rose-400 text-sm">
          {error}
        </div>
      )}

      <div className="bg-slate-900/80 border border-slate-800 rounded-2xl overflow-hidden backdrop-blur-sm">
        <div className="px-6 py-4 border-b border-slate-800 flex items-center justify-between">
          <h3 className="text-base font-semibold text-slate-100">Enrolled Endpoints ({agents.length})</h3>
          <span className="text-xs text-slate-400 font-mono">Live PostgreSQL Records</span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr className="border-b border-slate-800 bg-slate-950/40 text-xs font-medium text-slate-400 uppercase tracking-wider">
                <th className="py-3.5 px-6">Hostname</th>
                <th className="py-3.5 px-6">OS</th>
                <th className="py-3.5 px-6">Agent ID</th>
                <th className="py-3.5 px-6">IP Address</th>
                <th className="py-3.5 px-6">Status</th>
                <th className="py-3.5 px-6">Last Seen</th>
                <th className="py-3.5 px-6 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/80 text-sm text-slate-300">
              {agents.length === 0 && !loading ? (
                <tr>
                  <td colSpan="7" className="py-8 text-center text-slate-500 text-sm">
                    No agents enrolled yet. Click "Generate Enrollment Token" to register your first endpoint.
                  </td>
                </tr>
              ) : (
                agents.map((agent) => {
                  const state = getAgentState(agent);
                  return (
                    <tr key={agent.agent_id} className="hover:bg-slate-800/40 transition-colors">
                      <td className="py-4 px-6 font-semibold text-slate-100 flex items-center gap-2.5">
                        <Monitor className="h-4 w-4 text-indigo-400 shrink-0" />
                        {agent.hostname}
                      </td>
                      <td className="py-4 px-6">
                        <span className="text-xs px-2 py-0.5 rounded bg-slate-800 text-slate-300 font-mono uppercase">
                          {agent.os}
                        </span>
                      </td>
                      <td
                        className="py-4 px-6 font-mono text-xs text-slate-400 truncate max-w-[140px]"
                        title={agent.agent_id}
                      >
                        {agent.agent_id}
                      </td>
                      <td className="py-4 px-6 font-mono text-xs text-slate-300">
                        {agent.ip_address || "—"}
                      </td>
                      <td className="py-4 px-6">
                        <span
                          className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium ${
                            state === "online"
                              ? "bg-emerald-500/10 text-emerald-400 border border-emerald-500/20"
                              : state === "revoked"
                              ? "bg-rose-500/10 text-rose-400 border border-rose-500/20"
                              : "bg-amber-500/10 text-amber-400 border border-amber-500/20"
                          }`}
                        >
                          <span
                            className={`h-1.5 w-1.5 rounded-full ${
                              state === "online"
                                ? "bg-emerald-400 animate-pulse"
                                : state === "revoked"
                                ? "bg-rose-400"
                                : "bg-amber-400"
                            }`}
                          />
                          {state.toUpperCase()}
                        </span>
                      </td>
                      <td className="py-4 px-6 text-xs text-slate-400" title={formatDate(agent.last_seen_at)}>
                        {relativeTime(agent.last_seen_at)}
                      </td>
                      <td className="py-4 px-6 text-right space-x-2">
                        <button
                          onClick={() => handleToggleRevoke(agent)}
                          className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
                            agent.status === "revoked"
                              ? "bg-emerald-500/10 border border-emerald-500/30 text-emerald-300 hover:bg-emerald-500/20"
                              : "bg-amber-500/10 border border-amber-500/30 text-amber-300 hover:bg-amber-500/20"
                          }`}
                        >
                          {agent.status === "revoked" ? (
                            <ShieldCheck className="h-3.5 w-3.5" />
                          ) : (
                            <ShieldAlert className="h-3.5 w-3.5" />
                          )}
                          {agent.status === "revoked" ? "Unrevoke" : "Revoke"}
                        </button>
                        <button
                          onClick={() => handleDelete(agent.agent_id)}
                          className="p-1.5 text-slate-400 hover:text-rose-400 hover:bg-rose-500/10 rounded-lg transition-colors inline-block"
                          title="Delete Agent"
                        >
                          <Trash2 className="h-4 w-4" />
                        </button>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>

      <GenerateTokenModal isOpen={isTokenModalOpen} onClose={() => setIsTokenModalOpen(false)} />
    </div>
  );
};