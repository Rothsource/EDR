import React from 'react';
import { NavLink, useNavigate } from 'react-router-dom';
import { 
  Shield, 
  LayoutDashboard, 
  Monitor, 
  Activity, 
  AlertTriangle,
  Search, 
  ShieldCheck, 
  FileSpreadsheet, 
  Settings, 
  LogOut 
} from 'lucide-react';
import { useAuth } from '../context/AuthContext';

export const Sidebar = () => {
  const { logout, user } = useAuth();
  const navigate = useNavigate();

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  const navItems = [
    { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
    { to: '/agents', icon: Monitor, label: 'Agents Fleet' },
    { to: '/events', icon: Activity, label: 'Telemetry Stream' },
    { to: '/alerts', icon: AlertTriangle, label: 'Alerts & Incidents' },
    { to: '/threat-hunting', icon: Search, label: 'Threat Hunting' },
    { to: '/rules', icon: ShieldCheck, label: 'Detection Rules' },
    { to: '/compliance', icon: FileSpreadsheet, label: 'ISO 27001 & Compliance' },
    { to: '/settings', icon: Settings, label: 'Settings & Security' },
  ];

  return (
    <aside className="w-68 bg-slate-900 border-r border-slate-800 flex flex-col justify-between shrink-0 select-none">
      <div>
        <div className="p-6 border-b border-slate-800/80 flex items-center gap-3">
          <div className="h-10 w-10 rounded-xl bg-indigo-600/20 border border-indigo-500/30 flex items-center justify-center text-indigo-400">
            <Shield className="h-6 w-6" />
          </div>
          <div>
            <h1 className="font-bold text-slate-100 tracking-wide text-base">KhemStrix EDR</h1>
            <span className="text-[10px] text-indigo-400 font-mono tracking-wider">SOVEREIGN SOC V0.1</span>
          </div>
        </div>

        <nav className="p-4 space-y-1.5" aria-label="Main Navigation">
          {navItems.map((item) => {
            const Icon = item.icon;
            return (
              <NavLink
                key={item.to}
                to={item.to}
                className={({ isActive }) =>
                  `flex items-center gap-3 px-3.5 py-2.5 rounded-lg text-sm font-medium transition-colors ${
                    isActive
                      ? 'bg-indigo-600 text-white shadow-lg shadow-indigo-600/20'
                      : 'text-slate-300 hover:text-white hover:bg-slate-800/60'
                  }`
                }
              >
                <Icon className="h-4 w-4 shrink-0" />
                <span className="truncate">{item.label}</span>
              </NavLink>
            );
          })}
        </nav>
      </div>

      <div className="p-4 border-t border-slate-800/80 bg-slate-950/40">
        <div className="flex items-center gap-2.5 mb-3 px-2">
          <div className="h-8 w-8 rounded-full bg-slate-800 border border-slate-700 flex items-center justify-center text-slate-200 font-bold text-xs shrink-0">
            {user?.username ? user.username.substring(0, 2).toUpperCase() : 'AD'}
          </div>
          <div className="truncate">
            <p className="text-xs font-medium text-slate-200 truncate">{user?.username || 'Administrator'}</p>
            <p className="text-[10px] text-indigo-400 truncate font-mono">Tenant: 0001</p>
          </div>
        </div>
        <button
          onClick={handleLogout}
          aria-label="Sign out of console"
          className="w-full flex items-center justify-center gap-2 px-3 py-2 rounded-lg text-xs font-medium text-rose-400 hover:text-rose-300 hover:bg-rose-500/10 border border-rose-500/20 transition-colors"
        >
          <LogOut className="h-3.5 w-3.5" />
          Sign Out
        </button>
      </div>
    </aside>
  );
};

export default Sidebar;