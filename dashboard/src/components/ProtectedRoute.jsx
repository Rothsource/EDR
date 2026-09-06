import { Navigate } from "react-router-dom";
import { useAuth } from "../context/AuthContext";

// Frontend-side guard only — a UX convenience so the app doesn't flash
// protected content before an API call fails. The real security boundary
// is the backend's JWT check (core/deps.py -> get_current_user_id).
export default function ProtectedRoute({ children }) {
  const { isAuthenticated } = useAuth();
  if (!isAuthenticated) return <Navigate to="/login" replace />;
  return children;
}
