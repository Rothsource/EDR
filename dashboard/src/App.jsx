import React from 'react';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { AuthProvider } from './context/AuthContext';
import { ProtectedRoute } from './components/ProtectedRoute';
import { AppShell } from './components/AppShell';
import { Dashboard } from './pages/Dashboard';
import { Agents } from './pages/Agents';
import { Events } from './pages/Events';
import { Alerts } from './pages/Alerts';
import { ThreatHunting } from './pages/ThreatHunting';
import { Rules } from './pages/Rules';
import { Compliance } from './pages/Compliance';
import { Settings } from './pages/Settings';
import { Login } from './pages/Login';

export function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route element={<ProtectedRoute />}>
            <Route element={<AppShell />}>
              <Route path="/" element={<Dashboard />} />
              <Route path="/agents" element={<Agents />} />
              <Route path="/events" element={<Events />} />
              <Route path="/alerts" element={<Alerts />} />
              <Route path="/threat-hunting" element={<ThreatHunting />} />
              <Route path="/rules" element={<Rules />} />
              <Route path="/compliance" element={<Compliance />} />
              <Route path="/settings" element={<Settings />} />
            </Route>
          </Route>
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  );
}

export default App;