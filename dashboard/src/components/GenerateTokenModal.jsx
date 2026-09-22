import React, { useState, useEffect } from "react";
import { X, Copy, Check, Key, Terminal, Monitor, RefreshCw, AlertCircle } from "lucide-react";
import { api, API_URL } from "../api";

export const GenerateTokenModal = ({ isOpen, onClose }) => {
  const [copied, setCopied] = useState(false);
  const [osType, setOsType] = useState("linux");
  const [tokenData, setTokenData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  const fetchToken = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.generateToken();
      setTokenData(data);
    } catch (err) {
      setError(err.message || "Failed to generate enrollment token");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (isOpen) {
      fetchToken();
    }
  }, [isOpen]);

  if (!isOpen) return null;

  const token = tokenData?.token || "";
  const expiresAt = tokenData?.expires_at ? new Date(tokenData.expires_at).toLocaleTimeString() : "1 hour";

  // Exact CLI commands matching your original implementation
  const getCommand = () => {
    if (osType === "windows") {
      return `Invoke-WebRequest -Uri "${API_URL}/download/agent/windows" -OutFile "$env:TEMP\\khemstrixAgent.exe"; & "$env:TEMP\\khemstrixAgent.exe" --server=${API_URL} --token=${token}`;
    }
    return `curl -o /tmp/khemstrixAgent ${API_URL}/download/agent/linux && chmod +x /tmp/khemstrixAgent && sudo /tmp/khemstrixAgent --server=${API_URL} --token=${token}`;
  };

  const cliCommand = getCommand();

  const handleCopy = () => {
    navigator.clipboard.writeText(cliCommand);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div 
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/80 backdrop-blur-sm p-4"
      onClick={onClose}
    >
      <div 
        className="bg-slate-900 border border-slate-800 rounded-2xl w-full max-w-xl shadow-2xl overflow-hidden animate-in fade-in zoom-in-95 duration-150"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-slate-800">
          <div className="flex items-center gap-3">
            <div className="h-9 w-9 rounded-xl bg-indigo-600/20 border border-indigo-500/30 flex items-center justify-center text-indigo-400">
              <Key className="h-5 w-5" />
            </div>
            <div>
              <h3 className="text-base font-semibold text-slate-100">Enroll New Endpoint</h3>
              <p className="text-xs text-slate-400">Single-use token valid until {expiresAt}</p>
            </div>
          </div>
          <button 
            onClick={onClose}
            aria-label="Close modal"
            className="text-slate-400 hover:text-slate-200 p-1.5 rounded-lg hover:bg-slate-800 transition-colors"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="p-6 space-y-5">
          {error && (
            <div className="p-3 bg-rose-500/10 border border-rose-500/20 rounded-xl text-rose-400 text-xs flex items-center gap-2">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          {/* OS Selector Tabs */}
          <div>
            <label className="block text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">
              Select Target OS
            </label>
            <div className="grid grid-cols-2 gap-3">
              <button
                type="button"
                onClick={() => setOsType("linux")}
                className={`flex items-center justify-center gap-2 py-2.5 px-4 rounded-xl border text-xs font-medium transition-all ${
                  osType === "linux"
                    ? "bg-indigo-600/20 border-indigo-500 text-indigo-300 shadow-sm"
                    : "bg-slate-950/60 border-slate-800 text-slate-400 hover:bg-slate-800/40 hover:text-slate-200"
                }`}
              >
                <Terminal className="h-4 w-4" />
                Linux / Kali (curl)
              </button>

              <button
                type="button"
                onClick={() => setOsType("windows")}
                className={`flex items-center justify-center gap-2 py-2.5 px-4 rounded-xl border text-xs font-medium transition-all ${
                  osType === "windows"
                    ? "bg-indigo-600/20 border-indigo-500 text-indigo-300 shadow-sm"
                    : "bg-slate-950/60 border-slate-800 text-slate-400 hover:bg-slate-800/40 hover:text-slate-200"
                }`}
              >
                <Monitor className="h-4 w-4" />
                Windows (PowerShell)
              </button>
            </div>
          </div>

          {/* Raw Token Display */}
          <div className="bg-slate-950/70 border border-slate-800 rounded-xl p-3.5 space-y-1.5">
            <div className="flex items-center justify-between">
              <span className="text-[11px] font-semibold text-slate-400 uppercase tracking-wider">
                Enrollment Secret
              </span>
              <button
                onClick={fetchToken}
                disabled={loading}
                className="text-[11px] text-indigo-400 hover:text-indigo-300 flex items-center gap-1 font-mono hover:underline disabled:opacity-50"
              >
                <RefreshCw className={`h-3 w-3 ${loading ? "animate-spin" : ""}`} />
                Regenerate
              </button>
            </div>
            <input
              type="text"
              readOnly
              value={loading ? "Generating token..." : token}
              className="w-full bg-slate-900 border border-slate-800 rounded-lg px-3 py-1.5 text-xs font-mono text-indigo-300 outline-none select-all"
            />
          </div>

          {/* Terminal CLI Command Snippet */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold text-slate-300 uppercase tracking-wider flex items-center gap-1.5">
                <Terminal className="h-3.5 w-3.5 text-indigo-400" />
                Terminal Installation Command
              </span>
              <button
                onClick={handleCopy}
                disabled={loading || !token}
                aria-label="Copy CLI commands"
                className="flex items-center gap-1.5 px-3 py-1 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 text-white rounded-lg text-xs font-medium transition-colors shadow-sm"
              >
                {copied ? <Check className="h-3.5 w-3.5 text-emerald-300" /> : <Copy className="h-3.5 w-3.5" />}
                {copied ? "Copied" : "Copy CLI"}
              </button>
            </div>

            <div className="relative group">
              <pre className="bg-slate-950 border border-slate-800 rounded-xl p-4 text-xs font-mono text-emerald-400 overflow-x-auto whitespace-pre-wrap leading-relaxed select-all">
                {loading ? "# Requesting token from backend..." : cliCommand}
              </pre>
            </div>
            <p className="text-[11px] text-slate-500">
              Run this command on the target machine as Administrator/root to automatically download and enroll the agent[cite: 3].
            </p>
          </div>

          {/* Footer Action */}
          <div className="pt-3 border-t border-slate-800 flex justify-end">
            <button
              onClick={onClose}
              className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-xl text-xs font-medium transition-colors"
            >
              Done
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default GenerateTokenModal;