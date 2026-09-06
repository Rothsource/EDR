import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../context/AuthContext";

export default function Login() {
  const { login } = useAuth();
  const navigate = useNavigate();

  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      await login(username, password);
      navigate("/");
    } catch (err) {
      // Backend intentionally returns the same generic message whether the
      // username or the password was wrong — we just surface it as-is.
      setError(err.message || "Invalid credentials.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-background p-4">
      <div className="w-full max-w-md bg-surface rounded-xl shadow-md p-8">
        <div className="flex items-center gap-3 mb-6">
          <div className="w-10 h-10 rounded-lg bg-primary flex items-center justify-center text-on-primary shadow-sm">
            <span className="material-symbols-outlined text-[24px]">shield_person</span>
          </div>
          <div>
            <div className="flex items-baseline gap-1.5">
              <span className="font-bold text-lg text-on-surface">EDR</span>
              <span className="font-medium text-lg text-on-surface-variant">Console</span>
            </div>
            <p className="text-xs text-on-surface-variant">
              Internal endpoint monitoring — admin access
            </p>
          </div>
        </div>

        {error && (
          <div className="mb-4 flex items-start gap-2 p-3 rounded-lg bg-error-container text-on-error-container">
            <span className="material-symbols-outlined text-[18px] mt-0.5 text-error shrink-0">
              error
            </span>
            <p className="text-sm leading-snug">{error}</p>
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1">
            <label htmlFor="username" className="text-sm font-medium text-on-surface">
              Username
            </label>
            <div className="relative flex items-center">
              <span className="material-symbols-outlined absolute left-3 text-outline text-[20px]">
                person
              </span>
              <input
                id="username"
                type="text"
                required
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="w-full h-10 pl-10 pr-3 rounded-lg bg-surface-container-low text-on-surface text-sm outline-none focus:ring-2 focus:ring-primary/40 transition"
                placeholder="admin"
              />
            </div>
          </div>

          <div className="flex flex-col gap-1">
            <label htmlFor="password" className="text-sm font-medium text-on-surface">
              Password
            </label>
            <div className="relative flex items-center">
              <span className="material-symbols-outlined absolute left-3 text-outline text-[20px]">
                lock
              </span>
              <input
                id="password"
                type={showPassword ? "text" : "password"}
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="w-full h-10 pl-10 pr-10 rounded-lg bg-surface-container-low text-on-surface text-sm outline-none focus:ring-2 focus:ring-primary/40 transition"
                placeholder="••••••••••••"
              />
              <button
                type="button"
                aria-label="Toggle password visibility"
                onClick={() => setShowPassword((v) => !v)}
                className="absolute right-2 p-1 text-outline hover:text-on-surface rounded"
              >
                <span className="material-symbols-outlined text-[20px]">
                  {showPassword ? "visibility_off" : "visibility"}
                </span>
              </button>
            </div>
          </div>

          <button
            type="submit"
            disabled={loading}
            className="mt-2 w-full h-10 rounded-lg bg-primary text-on-primary text-sm font-medium flex items-center justify-center gap-2 shadow-sm hover:opacity-95 active:scale-[0.99] transition disabled:opacity-60"
          >
            {loading ? "Logging in…" : "Log in"}
            {!loading && <span className="material-symbols-outlined text-[18px]">arrow_forward</span>}
          </button>
        </form>
      </div>
    </div>
  );
}
