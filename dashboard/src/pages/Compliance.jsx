import React from 'react';
import { PageHeader } from '../components/PageHeader';
import { CheckCircle2, Download } from 'lucide-react';

export const Compliance = () => {
  const controls = [
    { code: 'ISO 27001 A.12.4.1', title: 'Event Logging & Audit Trails', status: 'Compliant', detail: 'Automated WebSocket telemetry collection and 24h Postgres retention' },
    { code: 'ISO 27001 A.12.6.1', title: 'Management of Technical Vulnerabilities', status: 'Compliant', detail: 'Real-time endpoint process and package auditing via Kali/Windows agents' },
    { code: 'MPTC Cyber Law Sec 4', title: 'Sovereign Data Residency', status: 'Compliant', detail: 'All telemetry stored locally in on-premise PostgreSQL (Model 1)' },
    { code: 'NBC-TCRMG', title: 'Incident Response & Isolation', status: 'Compliant', detail: 'Active response containment and host isolation tested & verified' },
  ];

  return (
    <div>
      <PageHeader
        title="ISO 27001 & Regulatory Compliance"
        description="Audit-ready compliance templates aligned with MPTC Cybersecurity Law and National Bank of Cambodia guidelines."
        action={
          <button className="flex items-center gap-2 px-4 py-2.5 bg-indigo-600 hover:bg-indigo-500 text-white rounded-xl text-sm font-medium transition-colors shadow-lg shadow-indigo-600/20">
            <Download className="h-4 w-4" />
            Export Compliance CSV
          </button>
        }
      />

      <div className="bg-slate-900/80 border border-slate-800 rounded-2xl overflow-hidden backdrop-blur-sm mb-8">
        <div className="px-6 py-4 border-b border-slate-800">
          <h3 className="text-base font-semibold text-slate-100">Security Gap Assessment Matrix</h3>
        </div>

        <div className="divide-y divide-slate-800/80">
          {controls.map((c, i) => (
            <div key={i} className="p-5 flex items-center justify-between hover:bg-slate-800/30 transition-colors">
              <div className="flex items-start gap-4">
                <div className="mt-1 p-2 rounded-xl bg-emerald-500/10 border border-emerald-500/30 text-emerald-400">
                  <CheckCircle2 className="h-5 w-5" />
                </div>
                <div>
                  <div className="flex items-center gap-3">
                    <span className="font-mono text-xs text-indigo-300 bg-indigo-500/10 border border-indigo-500/20 px-2 py-0.5 rounded">{c.code}</span>
                    <h4 className="font-semibold text-slate-100 text-sm">{c.title}</h4>
                  </div>
                  <p className="text-xs text-slate-400 mt-1">{c.detail}</p>
                </div>
              </div>

              <div>
                <span className="px-3 py-1 rounded-full bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 text-xs font-medium">
                  {c.status}
                </span>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};
