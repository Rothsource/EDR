import { createContext, useContext, useState, useCallback } from "react";
import { api, getToken, setToken, clearToken } from "../api";

const AuthContext = createContext(null);

export function AuthProvider({ children }) {
  const [isAuthenticated, setIsAuthenticated] = useState(!!getToken());

  const login = useCallback(async (username, password) => {
    const data = await api.login(username, password);
    setToken(data.access_token);
    setIsAuthenticated(true);
  }, []);

  const logout = useCallback(() => {
    // Stateless JWT — logout is purely a client-side action.
    // There is nothing to tell the server; the token just gets discarded.
    clearToken();
    setIsAuthenticated(false);
  }, []);

  // Called by pages when an API call throws AuthError (expired/invalid token).
  const forceLogout = useCallback(() => {
    clearToken();
    setIsAuthenticated(false);
  }, []);

  return (
    <AuthContext.Provider value={{ isAuthenticated, login, logout, forceLogout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  return useContext(AuthContext);
}
