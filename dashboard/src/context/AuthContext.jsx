import React, { createContext, useContext, useState, useEffect } from "react";
import { api, getToken, setToken, clearToken, AuthError } from "../api";

const AuthContext = createContext(null);

export const AuthProvider = ({ children }) => {
  const [token, setAuthTokenState] = useState(getToken());
  const [user, setUser] = useState({
    username: localStorage.getItem("edr_username") || "admin",
    tenant: "00000000-0000-0000-0000-000000000001",
  });

  useEffect(() => {
    const currentToken = getToken();
    setAuthTokenState(currentToken);
  }, []);

  const login = async (username, password) => {
    try {
      const data = await api.login(username, password);
      if (data && data.access_token) {
        setToken(data.access_token);
        localStorage.setItem("edr_username", username);
        setAuthTokenState(data.access_token);
        setUser((prev) => ({ ...prev, username }));
        return true;
      }
    } catch (err) {
      if (err instanceof AuthError) {
        throw new Error("Invalid username or password");
      }
      throw err;
    }
    return false;
  };

  const logout = () => {
    clearToken();
    localStorage.removeItem("edr_username");
    setAuthTokenState(null);
  };

  return (
    <AuthContext.Provider
      value={{
        token,
        user,
        login,
        logout,
        isAuthenticated: !!token,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = () => useContext(AuthContext);