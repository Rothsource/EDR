// Central place for every call to your FastAPI backend.
export const API_URL = import.meta.env.VITE_API_URL || `http://${window.location.hostname}:8000`;

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

  unrevokeAgent: (agentId) =>
    request(`/admin/agents/${agentId}/unrevoke`, { method: "PATCH" }),

  deleteAgent: (agentId) =>
    request(`/admin/agents/${agentId}`, { method: "DELETE" }),

  listEvents: (params = {}) => {
    const qs = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined && v !== null && v !== "") qs.set(k, v);
    });
    const suffix = qs.toString();
    return request(`/events${suffix ? `?${suffix}` : ""}`);
  },
};