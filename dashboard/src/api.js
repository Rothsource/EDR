// Central place for every call to your FastAPI backend.
// Base URL comes from .env (VITE_API_URL) so it's easy to point at
// a different machine on your local network without touching code.
const API_URL = import.meta.env.VITE_API_URL || "http://localhost:8000";

const TOKEN_KEY = "edr_access_token";

export function getToken() {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token) {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

// Thrown when the backend says the token is missing/invalid/expired.
// Components can catch this specifically to redirect to /login.
export class AuthError extends Error {}

async function request(path, { method = "GET", body, auth = true } = {}) {
  const headers = { "Content-Type": "application/json" };

  if (auth) {
    const token = getToken();
    if (token) headers["Authorization"] = `Bearer ${token}`;
  }

  const res = await fetch(`${API_URL}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });

  // Parse JSON if there is any body, otherwise null.
  let data = null;
  const text = await res.text();
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = text;
    }
  }

  if (res.status === 401) {
    clearToken();
    throw new AuthError(data?.detail || "not authenticated");
  }

  if (!res.ok) {
    const message = data?.detail || `Request failed (${res.status})`;
    throw new Error(typeof message === "string" ? message : JSON.stringify(message));
  }

  return data;
}

export const api = {
  login: (username, password) =>
    request("/auth/login", { method: "POST", body: { username, password }, auth: false }),

  changePassword: (current_password, new_password) =>
    request("/auth/change-password", {
      method: "PUT",
      body: { current_password, new_password },
    }),

  listAgents: () => request("/agents"),

  generateToken: () => request("/admin/generate-token", { method: "POST" }),

  revokeAgent: (agentId) =>
    request(`/admin/agents/${agentId}/revoke`, { method: "PATCH" }),

  deleteAgent: (agentId) =>
    request(`/admin/agents/${agentId}`, { method: "DELETE" }),
};