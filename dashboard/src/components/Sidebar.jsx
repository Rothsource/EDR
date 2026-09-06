import { NavLink } from "react-router-dom";
import { useAuth } from "../context/AuthContext";

const navItems = [
  { to: "/", label: "Dashboard", icon: "home", end: true },
  { to: "/agents", label: "Agents", icon: "computer" },
  { to: "/settings", label: "Settings", icon: "settings" },
];

export default function Sidebar() {
  const { logout } = useAuth();

  return (
    <aside className="w-64 shrink-0 bg-surface border-r border-outline-variant flex flex-col h-screen sticky top-0">
      <div className="flex items-center gap-3 px-5 py-5">
        <div className="w-9 h-9 rounded-lg bg-primary flex items-center justify-center text-on-primary shadow-sm">
          <span className="material-symbols-outlined text-[20px]">shield_person</span>
        </div>
        <div className="flex flex-col leading-tight">
          <span className="font-semibold text-on-surface text-sm">EDR Console</span>
          <span className="text-[11px] uppercase tracking-wide text-outline">
            Security Admin
          </span>
        </div>
      </div>

      <nav className="flex-1 px-3 mt-2 flex flex-col gap-1">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            className={({ isActive }) =>
              `flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-colors ${
                isActive
                  ? "bg-surface-container text-primary"
                  : "text-on-surface-variant hover:bg-surface-container-low"
              }`
            }
          >
            <span className="material-symbols-outlined text-[20px]">{item.icon}</span>
            {item.label}
          </NavLink>
        ))}
      </nav>

      <div className="px-3 pb-5">
        <button
          onClick={logout}
          className="w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium text-on-surface-variant hover:bg-surface-container-low transition-colors"
        >
          <span className="material-symbols-outlined text-[20px]">logout</span>
          Log out
        </button>
      </div>
    </aside>
  );
}
