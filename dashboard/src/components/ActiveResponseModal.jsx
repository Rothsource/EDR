import React, { useState } from 'react';
import { X, Zap, Radio, CheckCircle2 } from 'lucide-react';

export const ActiveResponseModal = ({ isOpen, onClose, agent }) => {
  const [actionType, setActionType] = useState('isolate');
  const [executed, setExecuted] = useState(false);
  const [loading, setLoading] = useState(false);

  if (!isOpen || !agent) return null;

  const handleExecute = () => {
    setLoading(true);
    setTimeout(() => {
      setLoading(false);
      setExecuted(true);
    }, 800);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/80 backdrop-blur-sm p-4 animate-fadeIn">
      <div className="bg-slate-900 border border-slate-800 rounded-2xl w-full max-w-lg shadow-2xl overflow-hidden">
        <div className="flex items-center justify-between px-6 py-4 border-b border-slate-800 bg-rose-500/10">
          <div className="flex items-center gap-3">
            <div className="h-9 w-9 rounded-lg bg-rose-500/20 border border-rose-500/30 flex items-center justify-center text-rose-400">
              <Zap className="h-5 w-5" />
            </div>
            <div>
              <h3 className="text-base font-semibold text-slate-100">Active Response & Containment</h3>
              <p className="text-xs text-rose-300/80">Target: {agent.hostname} ({agent.ip})</p>
            </div>
          </div>
          <button onClick={onClose} className="text-slate-400 hover:text-slate-200 p-1 rounded-lg hover:bg-slate-800 transition-colors">
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="p-6 space-y-5">
          {executed ? (
            <div className="text-center py-6 space-y-3">
              <div className="h-12 w-12 rounded-full bg-emerald-500/20 border border-emerald-500/30 text-emerald-400 flex items-center justify-center mx-auto">
                <CheckCircle2 className="h-6 w-6" />
              </div>
              <h4 className="text-lg font-bold text-slate-100">Active Response Executed Successfully</h4>
              <p className="text-xs text-slate-400">Command dispatched over WebSocket to {agent.hostname}.</p>
              <button
                onClick={() => { setExecuted(false); onClose(); }}
                className="mt-4 px-6 py-2.5 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-xl text-sm font-medium transition-colors"
              >
                Dismiss
              </button>
            </div>
          ) : (
            <>
              <div className="space-y-3">
                <label className="block text-xs font-medium text-slate-400 uppercase tracking-wider">Select Containment Action</label>
                
                <div 
                  onClick={() => setActionType('isolate')}
                  className={`p-4 rounded-xl border cursor-pointer transition-all ${actionType === 'isolate' ? 'bg-rose-500/10 border-rose-500/40 text-slate-100' : 'bg-slate-950/60 border-slate-800 text-slate-400'}`}
                >
                  <div className="font-semibold text-sm mb-1 text-slate-200 flex items-center justify-between">
                    <span>Host Network Isolation</span>
                    <span className="text-[10px] font-mono bg-rose-500/20 text-rose-300 px-2 py-0.5 rounded">CrowdStrike Standard</span>
                  </div>
                  <p className="text-xs text-slate-400">Block all inbound/outbound TCP traffic on endpoint except control channel.</p>
                </div>

                <div 
                  onClick={() => setActionType('kill')}
                  className={`p-4 rounded-xl border cursor-pointer transition-all ${actionType === 'kill' ? 'bg-amber-500/10 border-amber-500/40 text-slate-100' : 'bg-slate-950/60 border-slate-800 text-slate-400'}`}
                >
                  <div className="font-semibold text-sm mb-1 text-slate-200 flex items-center justify-between">
                    <span>Kill Suspicious Process Tree</span>
                    <span className="text-[10px] font-mono bg-amber-500/20 text-amber-300 px-2 py-0.5 rounded">EDR Action</span>
                  </div>
                  <p className="text-xs text-slate-400">Terminate unauthorized child processes instantly.</p>
                </div>
              </div>

              <div className="pt-2 flex items-center justify-end gap-3 border-t border-slate-800">
                <button
                  onClick={onClose}
                  className="px-4 py-2.5 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-xl text-sm font-medium transition-colors"
                >
                  Cancel
                </button>
                <button
                  onClick={handleExecute}
                  disabled={loading}
                  className="px-6 py-2.5 bg-rose-600 hover:bg-rose-500 text-white rounded-xl text-sm font-semibold transition-colors shadow-lg shadow-rose-600/25 flex items-center gap-2"
                >
                  {loading ? 'Executing...' : 'Execute Action'}
                </button>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
};
