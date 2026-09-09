import { useState, useEffect } from "react";
import { api, AuthError, API_URL } from "../api";
import { useAuth } from "../context/AuthContext";

export default function GenerateTokenModal({ onClose }) {
  const { forceLogout } = useAuth();
  const [token, setToken] = useState(null);
  const [expiresAt, setExpiresAt] = useState(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [copied, setCopied] = useState(false);
  const [commandCopied, setCommandCopied] = useState(false);
  const [os, setOs] = useState("windows"); // "windows" | "linux"

  async function handleGenerate() {
    setLoading(true);
    setError("");
    try {
      const data = await api.generateToken();
      setToken(data.token);
      setExpiresAt(data.expires_at);
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

  // Generate immediately when the modal opens.
  useEffect(() => {
    handleGenerate();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function copyToken() {
    if (!token) return;
    navigator.clipboard.writeText(token);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  function buildInstallCommand() {
    if (!token) return "";
    if (os === "windows") {
      <p className="text-xs text-warning flex items-center gap-1 mb-2">
        <span className="material-symbols-outlined text-[14px]">warning</span>
        Must be run in an elevated (Administrator) PowerShell window
      </p>
      return `Invoke-WebRequest -Uri "${API_URL}/download/agent/windows" -OutFile "$env:TEMP\\khemstrixAgent.exe"; & "$env:TEMP\\khemstrixAgent.exe" --server=${API_URL} --token=${token}`;
    }
    return `curl -o /tmp/khemstrixAgent ${API_URL}/download/agent/linux && chmod +x /tmp/khemstrixAgent && sudo /tmp/khemstrixAgent --server=${API_URL} --token=${token}`;
  }

  function copyCommand() {
    const cmd = buildInstallCommand();
    if (!cmd) return;
    navigator.clipboard.writeText(cmd);
    setCommandCopied(true);
    setTimeout(() => setCommandCopied(false), 2000);
  }

  return (
    <div className="fixed inset-0 bg-on-surface/40 flex items-center justify-center z-50 p-4">
      <div className="w-full max-w-md bg-surface rounded-xl shadow-lg p-6">
        <div className="flex items-center justify-between mb-1">
          <h2 className="text-base font-semibold text-on-surface">New Enrollment Token</h2>
          <button
            onClick={onClose}
            className="text-outline hover:text-on-surface p-1 rounded"
            aria-label="Close"
          >
            <span className="material-symbols-outlined text-[20px]">close</span>
          </button>
        </div>
        <p className="text-sm text-on-surface-variant mb-4">
          Use this token once to register a new agent.
        </p>

        {error && (
          <div className="mb-4 p-3 rounded-lg bg-error-container text-on-error-container text-sm">
            {error}
          </div>
        )}

        {loading && !token && (
          <div className="h-24 rounded-lg bg-surface-container animate-pulse mb-4" />
        )}

        {token && (
          <>
            <div className="mb-1">
              <label className="text-xs uppercase tracking-wide text-outline">
                Generated token
              </label>
              <div className="mt-1 flex items-stretch gap-2">
                <div className="flex-1 h-10 px-3 rounded-lg bg-surface-container flex items-center overflow-x-auto">
                  <code className="font-mono text-sm text-on-surface whitespace-nowrap">
                    {token}
                  </code>
                </div>
                <button
                  onClick={copyToken}
                  className="h-10 px-3 rounded-lg bg-primary text-on-primary text-sm font-medium flex items-center gap-1.5 hover:opacity-95 transition shrink-0"
                >
                  <span className="material-symbols-outlined text-[16px]">
                    {copied ? "check" : "content_copy"}
                  </span>
                  {copied ? "Copied" : "Copy"}
                </button>
              </div>
            </div>

            <p className="text-xs text-warning flex items-center gap-1 mt-2 mb-4">
              <span className="material-symbols-outlined text-[14px]">schedule</span>
              Expires {expiresAt ? new Date(expiresAt).toLocaleString() : "in 1 hour"} • Single
              use only
            </p>

            <div className="bg-surface-container-low rounded-lg p-3">
              <div className="flex items-center justify-between mb-2">
                <p className="text-xs uppercase tracking-wide text-outline">
                  Run this on the target machine
                </p>
                <div className="flex gap-1">
                  <button
                    onClick={() => setOs("windows")}
                    className={`text-xs px-2 py-1 rounded ${
                      os === "windows"
                        ? "bg-primary text-on-primary"
                        : "text-on-surface-variant hover:bg-surface-container"
                    }`}
                  >
                    Windows
                  </button>
                  <button
                    onClick={() => setOs("linux")}
                    className={`text-xs px-2 py-1 rounded ${
                      os === "linux"
                        ? "bg-primary text-on-primary"
                        : "text-on-surface-variant hover:bg-surface-container"
                    }`}
                  >
                    Linux
                  </button>
                </div>
              </div>

              <code className="block font-mono text-xs text-on-surface break-all mb-2">
                {buildInstallCommand()}
              </code>

              <button
                onClick={copyCommand}
                className="w-full h-8 rounded-lg bg-primary text-on-primary text-xs font-medium flex items-center justify-center gap-1.5 hover:opacity-95 transition"
              >
                <span className="material-symbols-outlined text-[14px]">
                  {commandCopied ? "check" : "content_copy"}
                </span>
                {commandCopied ? "Copied" : "Copy command"}
              </button>
            </div>
          </>
        )}

        <div className="flex items-center justify-between mt-6">
          <button
            onClick={handleGenerate}
            disabled={loading}
            className="text-sm text-primary font-medium hover:underline disabled:opacity-50"
          >
            Generate another
          </button>
          <button
            onClick={onClose}
            className="h-9 px-4 rounded-lg bg-primary text-on-primary text-sm font-medium hover:opacity-95 transition"
          >
            Done
          </button>
        </div>
      </div>
    </div>
  );
}