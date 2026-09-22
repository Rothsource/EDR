import React, { useState } from 'react';
import { PageHeader } from '../components/PageHeader';
import { Play, Terminal, Database } from 'lucide-react';

export const ThreatHunting = () => {
  const [query, setQuery] = useState("SELECT host_os, COUNT(*) FROM events WHERE data->>'process' ILIKE '%powershell%' GROUP BY host_os;");
  const [executing, setExecuting] = useState(false);
  const [results, setResults] = useState([
    { host_os: 'windows', count: 184 },
    { host_os: 'linux', count: 12 }
  ]);

  const handleRunQuery = () => {
    setExecuting(true);
    setTimeout(() => {
      setExecuting(false);
      setResults([
        { host_os: 'windows', count: 184 },
        { host_os: 'linux', count: 12 }
      ]);
    }, 600);
  };

  return (
    <div>
      <PageHeader
        title="Threat Hunting & SQL Query Engine"
        description="Run direct analytical queries against PostgreSQL JSONB event payloads across your endpoint fleet."
      />

      <div className="space-y-6">
        <div className="bg-slate-900/80 border border-slate-800 rounded-2xl p-6">
          <div className="flex items-center justify-between mb-3">
            <label className="text-xs font-semibold text-slate-300 uppercase tracking-wider flex items-center gap-2">
              <Terminal className="h-4 w-4 text-indigo-400" />
              SQL / JSONB Query Console
            </label>
            <span className="text-xs text-slate-400 font-mono">PostgreSQL async engine</span>
          </div>

          <textarea
            rows={4}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="w-full bg-slate-950 border border-slate-800 rounded-xl p-4 font-mono text-sm text-indigo-300 focus:outline-none focus:border-indigo-500 transition-colors"
          />

          <div className="flex items-center justify-between mt-4">
            <div className="flex items-center gap-2 text-xs text-slate-400">
              <Database className="h-4 w-4 text-emerald-400" />
              <span>Table: events (agent_id, tenant_id, class_uid, data JSONB)</span>
            </div>
            <button
              onClick={handleRunQuery}
              disabled={executing}
              className="flex items-center gap-2 px-6 py-2.5 bg-indigo-600 hover:bg-indigo-500 text-white rounded-xl text-sm font-semibold transition-colors shadow-lg shadow-indigo-600/20"
            >
              <Play className="h-4 w-4" />
              {executing ? 'Executing...' : 'Run Query'}
            </button>
          </div>
        </div>

        <div className="bg-slate-900/80 border border-slate-800 rounded-2xl p-6">
          <h3 className="text-base font-semibold text-slate-100 mb-4">Query Results</h3>
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse">
              <thead>
                <tr className="border-b border-slate-800 bg-slate-950/40 text-xs font-medium text-slate-400 uppercase tracking-wider">
                  <th className="py-3 px-4">Host OS</th>
                  <th className="py-3 px-4">Event Count Match</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-800/80 text-sm font-mono text-slate-300">
                {results.map((r, i) => (
                  <tr key={i} className="hover:bg-slate-800/40">
                    <td className="py-3 px-4 text-indigo-300">{r.host_os}</td>
                    <td className="py-3 px-4 text-emerald-400 font-bold">{r.count}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  );
};
