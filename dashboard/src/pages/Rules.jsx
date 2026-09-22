import React, { useState } from 'react';
import { PageHeader } from '../components/PageHeader';
import { ShieldCheck, Plus } from 'lucide-react';

export const Rules = () => {
  const [rules, setRules] = useState([
    { id: 'rule-01', name: 'Suspicious PowerShell Encoded Command', severity: 'High', status: true, trigger: 'Process Start -> powershell.exe -enc' },
    { id: 'rule-02', name: 'Brute-Force SSH Login Attempts', severity: 'Critical', status: true, trigger: 'Auth Log -> 5 failed SSH attempts in 60s' },
    { id: 'rule-03', name: 'Unauthorized Port Scanning via Nmap', severity: 'Medium', status: true, trigger: 'Process Start -> nmap -sS' },
    { id: 'rule-04', name: 'Office App Spawning Command Interpreter', severity: 'Critical', status: false, trigger: 'Parent Process -> winword.exe / excel.exe' },
  ]);

  const toggleRule = (id) => {
    setRules(rules.map(r => r.id === id ? { ...r, status: !r.status } : r));
  };

  return (
    <div>
      <PageHeader
        title="Detection Rules & Sigma Engine"
        description="Configure behavioral detection rules and threshold alerts over incoming WebSocket telemetry streams."
      />

      <div className="bg-slate-900/80 border border-slate-800 rounded-2xl overflow-hidden backdrop-blur-sm">
        <div className="px-6 py-4 border-b border-slate-800 flex items-center justify-between">
          <h3 className="text-base font-semibold text-slate-100">Active Detection Rules ({rules.length})</h3>
          <button className="flex items-center gap-1.5 px-3.5 py-2 bg-indigo-600 hover:bg-indigo-500 text-white rounded-xl text-xs font-medium transition-colors shadow-lg shadow-indigo-600/20">
            <Plus className="h-3.5 w-3.5" />
            Add Custom Rule
          </button>
        </div>

        <div className="divide-y divide-slate-800/80">
          {rules.map((rule) => (
            <div key={rule.id} className="p-5 flex items-center justify-between hover:bg-slate-800/30 transition-colors">
              <div className="flex items-start gap-4">
                <div className={`mt-1 p-2 rounded-xl border ${
                  rule.severity === 'Critical' ? 'bg-rose-500/10 border-rose-500/30 text-rose-400' :
                  rule.severity === 'High' ? 'bg-amber-500/10 border-amber-500/30 text-amber-400' :
                  'bg-blue-500/10 border-blue-500/30 text-blue-400'
                }`}>
                  <ShieldCheck className="h-5 w-5" />
                </div>
                <div>
                  <div className="flex items-center gap-3">
                    <h4 className="font-semibold text-slate-100 text-sm">{rule.name}</h4>
                    <span className={`text-[10px] px-2 py-0.5 rounded font-mono uppercase ${
                      rule.severity === 'Critical' ? 'bg-rose-500/20 text-rose-300' :
                      rule.severity === 'High' ? 'bg-amber-500/20 text-amber-300' :
                      'bg-blue-500/20 text-blue-300'
                    }`}>
                      {rule.severity}
                    </span>
                  </div>
                  <p className="text-xs text-slate-400 font-mono mt-1">Trigger: {rule.trigger}</p>
                </div>
              </div>

              <div className="flex items-center gap-4">
                <button
                  onClick={() => toggleRule(rule.id)}
                  className="flex items-center gap-2 text-xs font-medium transition-colors"
                >
                  {rule.status ? (
                    <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-emerald-500/10 border border-emerald-500/20 text-emerald-400">
                      <span className="h-1.5 w-1.5 rounded-full bg-emerald-400 animate-pulse" />
                      Enabled
                    </span>
                  ) : (
                    <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-slate-800 text-slate-400">
                      Disabled
                    </span>
                  )}
                </button>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};
