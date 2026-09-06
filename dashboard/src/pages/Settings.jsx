import { useState } from "react";
import { api, AuthError } from "../api";
import { useAuth } from "../context/AuthContext";
import AppShell from "../components/AppShell";
import PageHeader from "../components/PageHeader";

export default function Settings() {
  const { forceLogout } = useAuth();

  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [showCurrent, setShowCurrent] = useState(false);
  const [showNew, setShowNew] = useState(false);

  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e) {
    e.preventDefault();
    setError("");
    setSuccess("");

    if (newPassword !== confirmPassword) {
      setError("New password and confirmation do not match.");
      return;
    }

    setLoading(true);
    try {
      await api.changePassword(currentPassword, newPassword);
      setSuccess("Password updated successfully.");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
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

  return (
    <AppShell>
      <PageHeader title="Settings" subtitle="Manage your admin account" />

      <div className="max-w-lg bg-surface rounded-xl shadow-sm border border-outline-variant p-6">
        <h2 className="text-sm font-semibold text-on-surface mb-1 flex items-center gap-2">
          <span className="material-symbols-outlined text-[18px] text-primary">lock_reset</span>
          Change Password
        </h2>
        <p className="text-sm text-on-surface-variant mb-4">
          You'll need your current password to set a new one.
        </p>

        {success && (
          <div className="mb-4 p-3 rounded-lg bg-success-container text-success text-sm flex items-center gap-2">
            <span className="material-symbols-outlined text-[16px]">check_circle</span>
            {success}
          </div>
        )}
        {error && (
          <div className="mb-4 p-3 rounded-lg bg-error-container text-on-error-container text-sm flex items-center gap-2">
            <span className="material-symbols-outlined text-[16px]">error</span>
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <PasswordField
            label="Current Password"
            value={currentPassword}
            onChange={setCurrentPassword}
            show={showCurrent}
            onToggleShow={() => setShowCurrent((v) => !v)}
          />
          <PasswordField
            label="New Password"
            value={newPassword}
            onChange={setNewPassword}
            show={showNew}
            onToggleShow={() => setShowNew((v) => !v)}
          />
          <PasswordField
            label="Confirm New Password"
            value={confirmPassword}
            onChange={setConfirmPassword}
            show={showNew}
            onToggleShow={() => setShowNew((v) => !v)}
          />

          <div className="flex justify-end mt-2">
            <button
              type="submit"
              disabled={loading}
              className="h-9 px-4 rounded-lg bg-primary text-on-primary text-sm font-medium hover:opacity-95 transition disabled:opacity-60"
            >
              {loading ? "Updating…" : "Update Password"}
            </button>
          </div>
        </form>
      </div>
    </AppShell>
  );
}

function PasswordField({ label, value, onChange, show, onToggleShow }) {
  return (
    <div className="flex flex-col gap-1">
      <label className="text-sm font-medium text-on-surface">{label}</label>
      <div className="relative flex items-center">
        <span className="material-symbols-outlined absolute left-3 text-outline text-[18px]">
          lock
        </span>
        <input
          type={show ? "text" : "password"}
          required
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="w-full h-10 pl-10 pr-10 rounded-lg bg-surface-container-low text-sm outline-none focus:ring-2 focus:ring-primary/40 transition"
        />
        <button
          type="button"
          onClick={onToggleShow}
          className="absolute right-2 p-1 text-outline hover:text-on-surface rounded"
        >
          <span className="material-symbols-outlined text-[18px]">
            {show ? "visibility_off" : "visibility"}
          </span>
        </button>
      </div>
    </div>
  );
}
