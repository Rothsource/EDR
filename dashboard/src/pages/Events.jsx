import React, { useState, useEffect, useMemo, useRef, useCallback } from "react";
import { PageHeader } from "../components/PageHeader";
import { 
  Search, 
  RefreshCw, 
  AlertCircle, 
  X, 
  Terminal, 
  Copy, 
  Check, 
  Radio, 
  ArrowUpDown, 
  ChevronUp, 
  ChevronDown, 
  ChevronLeft, 
  ChevronRight, 
  ExternalLink,
  Filter
} from "lucide-react";
import { api } from "../api";
import { getSeverityBadge } from "../lib/severity";

function relativeTime(isoString) {
  if (!isoString) return "Never";
  const then = new Date(isoString).getTime();
  const diffSec = Math.floor((Date.now() - then) / 1000);
  if (diffSec < 5) return "Just now";
  if (diffSec < 60) return `${diffSec}s ago`;
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return `${diffHr}h ago`;
  return `${Math.floor(diffHr / 24)}d ago`;
}

// Syntax-highlighted OCSF JSON renderer
function SyntaxHighlightedJSON({ jsonString }) {
  const formatted = useMemo(() => {
    try {
      const parsed = typeof jsonString === "string" ? JSON.parse(jsonString) : jsonString;
      const str = JSON.stringify(parsed, null, 2);
      return str.replace(
        /("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+\-]?\d+)?)/g,
        (match) => {
          let cls = "text-emerald-400"; // number
          if (/^"/.test(match)) {
            if (/:$/.test(match)) {
              cls = "text-indigo-300 font-semibold"; // key
            } else {
              cls = "text-amber-300"; // string
            }
          } else if (/true|false/.test(match)) {
            cls = "text-rose-400 font-semibold"; // boolean
          } else if (/null/.test(match)) {
            cls = "text-slate-500 italic"; // null
          }
          return `<span class="${cls}">${match}</span>`;
        }
      );
    } catch {
      return jsonString;
    }
  }, [jsonString]);

  return (
    <pre 
      className="font-mono text-xs leading-relaxed text-slate-200 overflow-x-auto whitespace-pre-wrap select-text"
      dangerouslySetInnerHTML={{ __html: formatted }}
    />
  );
}

// Per-field copy pill with hover state
function FieldCopyPill({ label, value, onFilterSelect }) {
  const [copied, setCopied] = useState(false);
  if (!value) return null;

  const handleCopy = (e) => {
    e.stopPropagation();
    navigator.clipboard.writeText(String(value));
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <div className="group/pill flex items-center justify-between py-1 px-2 rounded-lg bg-slate-950/50 hover:bg-slate-900 border border-slate-800/80 text-xs">
      <span className="text-slate-400 font-medium">{label}:</span>
      <div className="flex items-center gap-1.5 ml-2">
        <span className="font-mono text-slate-200">{String(value)}</span>
        <button
          onClick={handleCopy}
          aria-label={`Copy ${label}`}
          className="opacity-0 group-hover/pill:opacity-100 p-1 hover:text-indigo-300 text-slate-400 transition-opacity"
        >
          {copied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
        </button>
        {onFilterSelect && (
          <button
            onClick={() => onFilterSelect(value)}
            aria-label={`Filter by ${label}`}
            className="opacity-0 group-hover/pill:opacity-100 p-1 hover:text-indigo-300 text-slate-400 transition-opacity"
            title="Filter by this value"
          >
            <Filter className="h-3 w-3" />
          </button>
        )}
      </div>
    </div>
  );
}

export const Events = () => {
  const [events, setEvents] = useState([]);
  const [pendingEvents, setPendingEvents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  // Filters
  const [filterOS, setFilterOS] = useState("all");
  const [filterReason, setFilterReason] = useState("all");
  const [filterService, setFilterService] = useState("all");
  const [searchTerm, setSearchTerm] = useState("");

  // Live Auto-Refresh State
  const [isLive, setIsLive] = useState(true);
  const [refreshIntervalSec, setRefreshIntervalSec] = useState(5);

  // Sorting
  const [sortField, setSortField] = useState("time");
  const [sortOrder, setSortOrder] = useState("desc"); // 'asc' | 'desc'

  // Selection & Inspector
  const [selectedIndex, setSelectedIndex] = useState(-1);
  const [selectedEvent, setSelectedEvent] = useState(null);
  const [copiedWhole, setCopiedWhole] = useState(false);

  const containerRef = useRef(null);

  const loadEvents = useCallback(async (isBackground = false) => {
    if (!isBackground) setLoading(true);
    setError(null);
    try {
      const res = await api.listEvents({ limit: 100 });
      const items = res?.items ? res.items : (Array.isArray(res) ? res : []);
      
      if (isBackground && !isLive) {
        // While paused, capture delta in pending pool
        setEvents((prev) => {
          const prevIds = new Set(prev.map((e) => e.event_id));
          const fresh = items.filter((e) => !prevIds.has(e.event_id));
          if (fresh.length > 0) setPendingEvents(fresh);
          return prev;
        });
      } else {
        setEvents(items);
        setPendingEvents([]);
      }
    } catch (err) {
      setError(err.message || "Failed to query telemetry events");
      if (!isBackground) setEvents([]);
    } finally {
      if (!isBackground) setLoading(false);
    }
  }, [isLive]);

  useEffect(() => {
    loadEvents();
  }, [loadEvents]);

  // Polling loop
  useEffect(() => {
    if (!isLive) return;
    const interval = setInterval(() => {
      loadEvents(true);
    }, refreshIntervalSec * 1000);
    return () => clearInterval(interval);
  }, [isLive, refreshIntervalSec, loadEvents]);

  // Merge pending events when user clicks pill
  const applyPendingEvents = () => {
    setEvents((prev) => [...pendingEvents, ...prev]);
    setPendingEvents([]);
  };

  // Filtered & Sorted list
  const filteredEvents = useMemo(() => {
    let result = events.filter((ev) => {
      const matchesOS = filterOS === "all" || ev.host_os?.toLowerCase() === filterOS.toLowerCase();
      const service = ev.data?.service;
      const reason = ev.data?.reason;

      const matchesService = filterService === "all" || service === filterService;
      const matchesReason = filterReason === "all" || reason === filterReason;

      const targetStr = `${ev.hostname || ""} ${ev.type_uid || ""} ${ev.username || ""} ${ev.data?.src_ip || ""} ${JSON.stringify(ev.data || {})}`.toLowerCase();
      const matchesSearch = targetStr.includes(searchTerm.toLowerCase());

      return matchesOS && matchesService && matchesReason && matchesSearch;
    });

    result.sort((a, b) => {
      let aVal = a[sortField];
      let bVal = b[sortField];

      if (sortField === "time") {
        aVal = new Date(aVal || 0).getTime();
        bVal = new Date(bVal || 0).getTime();
      }

      if (aVal < bVal) return sortOrder === "asc" ? -1 : 1;
      if (aVal > bVal) return sortOrder === "asc" ? 1 : -1;
      return 0;
    });

    return result;
  }, [events, filterOS, filterService, filterReason, searchTerm, sortField, sortOrder]);

  // Keep selectedEvent in sync with index
  useEffect(() => {
    if (selectedIndex >= 0 && selectedIndex < filteredEvents.length) {
      setSelectedEvent(filteredEvents[selectedIndex]);
    } else if (selectedIndex === -1) {
      setSelectedEvent(null);
    }
  }, [selectedIndex, filteredEvents]);

  // Keyboard navigation: Up/Down arrow, Enter, Esc
  useEffect(() => {
    const handleKeyDown = (e) => {
      if (["INPUT", "TEXTAREA"].includes(document.activeElement.tagName)) return;

      if (e.key === "ArrowDown") {
        e.preventDefault();
        setSelectedIndex((prev) => Math.min(prev + 1, filteredEvents.length - 1));
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        setSelectedIndex((prev) => Math.max(prev - 1, 0));
      } else if (e.key === "Enter" && selectedIndex >= 0) {
        e.preventDefault();
        setSelectedEvent(filteredEvents[selectedIndex]);
      } else if (e.key === "Escape") {
        setSelectedEvent(null);
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [filteredEvents, selectedIndex]);

  const handleToggleSort = (field) => {
    if (sortField === field) {
      setSortOrder((prev) => (prev === "asc" ? "desc" : "asc"));
    } else {
      setSortField(field);
      setSortOrder("desc");
    }
  };

  const handleCopyWholeJson = () => {
    if (!selectedEvent) return;
    navigator.clipboard.writeText(JSON.stringify(selectedEvent, null, 2));
    setCopiedWhole(true);
    setTimeout(() => setCopiedWhole(false), 2000);
  };

  // Related events client-side correlation
  const relatedCount = useMemo(() => {
    if (!selectedEvent) return 0;
    const ip = selectedEvent.data?.src_ip;
    const user = selectedEvent.username;
    return events.filter(
      (e) =>
        e.event_id !== selectedEvent.event_id &&
        ((ip && e.data?.src_ip === ip) || (user && e.username === user))
    ).length;
  }, [selectedEvent, events]);

  const activeFiltersCount = (filterOS !== "all" ? 1 : 0) + 
                             (filterReason !== "all" ? 1 : 0) + 
                             (filterService !== "all" ? 1 : 0) + 
                             (searchTerm ? 1 : 0);

  return (
    <div ref={containerRef} className="outline-none">
      <PageHeader
        title="Live Telemetry Stream"
        description="Low-latency OCSF stream with interactive filtering, per-field inspection, and live pause controls."
        action={
          <div className="flex items-center gap-3">
            {/* Live Toggle Pill */}
            <div className="flex items-center bg-slate-900 border border-slate-800 rounded-xl p-1">
              <button
                onClick={() => setIsLive(!isLive)}
                className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-medium transition-all ${
                  isLive
                    ? "bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 shadow-sm"
                    : "text-slate-400 hover:text-slate-200"
                }`}
                aria-label={isLive ? "Pause live streaming" : "Resume live streaming"}
              >
                <Radio className={`h-3.5 w-3.5 ${isLive ? "animate-pulse" : ""}`} />
                {isLive ? "Live Streaming" : "Stream Paused"}
              </button>
              {isLive && (
                <select
                  value={refreshIntervalSec}
                  onChange={(e) => setRefreshIntervalSec(Number(e.target.value))}
                  aria-label="Refresh Interval"
                  className="bg-transparent text-xs text-slate-400 hover:text-slate-200 font-mono px-2 py-1 outline-none cursor-pointer"
                >
                  <option value={3} className="bg-slate-900 text-slate-200">3s</option>
                  <option value={5} className="bg-slate-900 text-slate-200">5s</option>
                  <option value={10} className="bg-slate-900 text-slate-200">10s</option>
                </select>
              )}
            </div>

            <button
              onClick={() => loadEvents(false)}
              aria-label="Manual refresh telemetry events"
              className="p-2.5 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-xl transition-colors focus:ring-2 focus:ring-indigo-500"
              title="Force Refresh"
            >
              <RefreshCw className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
            </button>
          </div>
        }
      />

      {error && (
        <div className="mb-6 p-4 bg-rose-500/10 border border-rose-500/20 rounded-xl text-rose-400 text-sm flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <AlertCircle className="h-4 w-4 shrink-0" />
            <span>{error}</span>
          </div>
          <button onClick={() => loadEvents(false)} className="text-xs underline hover:text-rose-300">Retry</button>
        </div>
      )}

      {/* Floating Incoming Events Pill */}
      {pendingEvents.length > 0 && (
        <div className="mb-4 flex justify-center">
          <button
            onClick={applyPendingEvents}
            className="flex items-center gap-2 px-4 py-2 rounded-full bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold shadow-lg shadow-indigo-600/30 transition-all transform hover:-translate-y-0.5"
          >
            <span className="h-2 w-2 rounded-full bg-emerald-400 animate-ping" />
            {pendingEvents.length} new event{pendingEvents.length > 1 ? "s" : ""} received — click to display
          </button>
        </div>
      )}

      {/* Filter Toolbar */}
      <div className="bg-slate-900/80 border border-slate-800 rounded-2xl p-4 mb-6 space-y-3 backdrop-blur-sm">
        <div className="flex flex-col lg:flex-row items-stretch lg:items-center justify-between gap-3">
          {/* Search Box */}
          <div className="relative flex-1">
            <Search className="absolute left-3.5 top-2.5 h-4 w-4 text-slate-400" />
            <input
              type="text"
              placeholder="Search hostname, username, src_ip, UID, or payload..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className="w-full bg-slate-950 border border-slate-800 rounded-xl pl-10 pr-4 py-2 text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-indigo-500 transition-colors"
            />
          </div>

          {/* Quick Filter Selectors */}
          <div className="flex flex-wrap items-center gap-2 text-xs">
            {/* OS Filter */}
            <div className="flex items-center gap-1 bg-slate-950/80 border border-slate-800 rounded-lg px-2 py-1">
              <span className="text-slate-400 font-medium">OS:</span>
              <select
                value={filterOS}
                onChange={(e) => setFilterOS(e.target.value)}
                className="bg-transparent text-slate-200 font-medium outline-none cursor-pointer"
              >
                <option value="all" className="bg-slate-900">All</option>
                <option value="linux" className="bg-slate-900">Linux</option>
                <option value="windows" className="bg-slate-900">Windows</option>
              </select>
            </div>

            {/* Service Filter */}
            <div className="flex items-center gap-1 bg-slate-950/80 border border-slate-800 rounded-lg px-2 py-1">
              <span className="text-slate-400 font-medium">Service:</span>
              <select
                value={filterService}
                onChange={(e) => setFilterService(e.target.value)}
                className="bg-transparent text-slate-200 font-medium outline-none cursor-pointer"
              >
                <option value="all" className="bg-slate-900">All Services</option>
                <option value="sshd" className="bg-slate-900">sshd</option>
                <option value="sudo" className="bg-slate-900">sudo</option>
              </select>
            </div>

            {/* Failure Reason Filter */}
            <div className="flex items-center gap-1 bg-slate-950/80 border border-slate-800 rounded-lg px-2 py-1">
              <span className="text-slate-400 font-medium">Reason:</span>
              <select
                value={filterReason}
                onChange={(e) => setFilterReason(e.target.value)}
                className="bg-transparent text-slate-200 font-medium outline-none cursor-pointer"
              >
                <option value="all" className="bg-slate-900">All Reasons</option>
                <option value="bad_password" className="bg-slate-900">bad_password</option>
                <option value="unknown_user" className="bg-slate-900">unknown_user</option>
                <option value="sudo_bad_password" className="bg-slate-900">sudo_bad_password</option>
              </select>
            </div>
          </div>
        </div>

        {/* Removable Active Filter Chips */}
        {activeFiltersCount > 0 && (
          <div className="flex flex-wrap items-center gap-2 pt-2 border-t border-slate-800/60">
            <span className="text-[11px] text-slate-400 uppercase tracking-wider font-medium">Active:</span>
            {searchTerm && (
              <span className="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full bg-indigo-500/10 border border-indigo-500/20 text-indigo-300 text-xs">
                query: "{searchTerm}"
                <button onClick={() => setSearchTerm("")} className="hover:text-white"><X className="h-3 w-3" /></button>
              </span>
            )}
            {filterOS !== "all" && (
              <span className="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full bg-indigo-500/10 border border-indigo-500/20 text-indigo-300 text-xs">
                OS: {filterOS}
                <button onClick={() => setFilterOS("all")} className="hover:text-white"><X className="h-3 w-3" /></button>
              </span>
            )}
            {filterService !== "all" && (
              <span className="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full bg-indigo-500/10 border border-indigo-500/20 text-indigo-300 text-xs">
                service: {filterService}
                <button onClick={() => setFilterService("all")} className="hover:text-white"><X className="h-3 w-3" /></button>
              </span>
            )}
            {filterReason !== "all" && (
              <span className="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full bg-indigo-500/10 border border-indigo-500/20 text-indigo-300 text-xs">
                reason: {filterReason}
                <button onClick={() => setFilterReason("all")} className="hover:text-white"><X className="h-3 w-3" /></button>
              </span>
            )}
            <button
              onClick={() => {
                setSearchTerm("");
                setFilterOS("all");
                setFilterService("all");
                setFilterReason("all");
              }}
              className="text-xs text-slate-400 hover:text-rose-400 underline ml-2 transition-colors"
            >
              Clear all
            </button>
          </div>
        )}
      </div>

      {/* Events Table Container */}
      <div className="bg-slate-900/80 border border-slate-800 rounded-2xl overflow-hidden backdrop-blur-sm shadow-xl">
        <div className="px-6 py-3 border-b border-slate-800/80 flex items-center justify-between text-xs text-slate-400">
          <div className="flex items-center gap-3">
            <span className="font-semibold text-slate-200">
              Showing {filteredEvents.length} of {events.length} Telemetry Events
            </span>
            <span className="font-mono text-slate-500">Use ↑ ↓ arrows + Enter to inspect</span>
          </div>
          <span className="font-mono text-indigo-400">Collector: OCSF 3002</span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr className="border-b border-slate-800 bg-slate-950/60 text-xs font-semibold text-slate-300 uppercase tracking-wider">
                <th className="py-3 px-6 cursor-pointer select-none hover:text-white" onClick={() => handleToggleSort("time")}>
                  <div className="flex items-center gap-1.5">
                    <span>Time (UTC)</span>
                    {sortField === "time" ? (
                      sortOrder === "asc" ? <ChevronUp className="h-3.5 w-3.5 text-indigo-400" /> : <ChevronDown className="h-3.5 w-3.5 text-indigo-400" />
                    ) : (
                      <ArrowUpDown className="h-3 w-3 text-slate-600" />
                    )}
                  </div>
                </th>
                <th className="py-3 px-6 cursor-pointer select-none hover:text-white" onClick={() => handleToggleSort("severity_id")}>
                  <div className="flex items-center gap-1.5">
                    <span>Severity</span>
                    {sortField === "severity_id" ? (
                      sortOrder === "asc" ? <ChevronUp className="h-3.5 w-3.5 text-indigo-400" /> : <ChevronDown className="h-3.5 w-3.5 text-indigo-400" />
                    ) : (
                      <ArrowUpDown className="h-3 w-3 text-slate-600" />
                    )}
                  </div>
                </th>
                <th className="py-3 px-6">Host / Target User</th>
                <th className="py-3 px-6">Service / Source IP</th>
                <th className="py-3 px-6">Failure Reason</th>
                <th className="py-3 px-6">Type UID</th>
                <th className="py-3 px-6 text-right">Details</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/60 text-xs text-slate-300">
              {filteredEvents.length === 0 && !loading ? (
                <tr>
                  <td colSpan="7" className="py-12 text-center text-slate-400">
                    <p className="font-medium text-slate-300 text-sm">No telemetry records match your current filters</p>
                    <p className="text-xs text-slate-500 mt-1">Try broadening your search query or reset active filters.</p>
                  </td>
                </tr>
              ) : (
                filteredEvents.map((ev, idx) => {
                  const isSelected = selectedEvent?.event_id === ev.event_id || selectedIndex === idx;
                  const service = ev.data?.service || "—";
                  const srcIp = ev.data?.src_ip;
                  const reason = ev.data?.reason || "—";

                  return (
                    <tr
                      key={ev.event_id}
                      onClick={() => {
                        setSelectedIndex(idx);
                        setSelectedEvent(ev);
                      }}
                      className={`cursor-pointer transition-colors ${
                        isSelected
                          ? "bg-indigo-600/15 border-l-2 border-indigo-500"
                          : "hover:bg-slate-800/50"
                      }`}
                    >
                      <td className="py-3 px-6 font-mono text-slate-300 whitespace-nowrap">
                        {ev.time ? new Date(ev.time).toLocaleTimeString() : "—"}
                        <span className="block text-[10px] text-slate-500 font-sans">{relativeTime(ev.time)}</span>
                      </td>
                      <td className="py-3 px-6 whitespace-nowrap">
                        {getSeverityBadge(ev.severity_id)}
                      </td>
                      <td className="py-3 px-6">
                        <div className="font-semibold text-slate-100">{ev.hostname}</div>
                        <div className="text-[11px] text-slate-400 font-mono">
                          user: <span className="text-indigo-300">{ev.username || "unknown"}</span>
                        </div>
                      </td>
                      <td className="py-3 px-6 font-mono">
                        <span className="px-1.5 py-0.5 rounded bg-slate-800 text-slate-300 uppercase text-[10px]">
                          {service}
                        </span>
                        {srcIp ? (
                          <div className="text-slate-300 text-[11px] mt-0.5 flex items-center gap-1">
                            {srcIp}:{ev.data?.src_port || ""}
                          </div>
                        ) : (
                          <div className="text-slate-500 text-[11px] italic mt-0.5">local session</div>
                        )}
                      </td>
                      <td className="py-3 px-6">
                        <span className="font-mono text-amber-400 bg-amber-500/10 px-2 py-0.5 rounded border border-amber-500/20 text-[11px]">
                          {reason}
                        </span>
                      </td>
                      <td className="py-3 px-6 font-mono text-indigo-300 text-[11px]">
                        {ev.type_uid}
                      </td>
                      <td className="py-3 px-6 text-right whitespace-nowrap">
                        <span className="text-xs text-slate-400 hover:text-indigo-400 underline">Inspect</span>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Upgraded Slide-out Inspector Panel */}
      {selectedEvent && (
        <div 
          className="fixed inset-0 z-50 flex justify-end bg-slate-950/75 backdrop-blur-sm transition-opacity"
          onClick={() => setSelectedEvent(null)}
        >
          <div 
            className="w-full max-w-2xl bg-slate-900 border-l border-slate-800 h-full overflow-y-auto shadow-2xl p-6 flex flex-col justify-between"
            onClick={(e) => e.stopPropagation()}
          >
            <div>
              {/* Header & Prev/Next Toolbar */}
              <div className="flex items-center justify-between pb-4 border-b border-slate-800">
                <div className="flex items-center gap-3">
                  <div className="p-2.5 rounded-xl bg-indigo-500/10 border border-indigo-500/20 text-indigo-400">
                    <Terminal className="h-5 w-5" />
                  </div>
                  <div>
                    <h3 className="text-base font-semibold text-slate-100">Telemetry Event Inspector</h3>
                    <span className="text-xs font-mono text-slate-400 truncate max-w-xs block">
                      {selectedEvent.event_id}
                    </span>
                  </div>
                </div>

                <div className="flex items-center gap-1">
                  {/* Step Prev */}
                  <button
                    disabled={selectedIndex <= 0}
                    onClick={() => setSelectedIndex((prev) => Math.max(prev - 1, 0))}
                    aria-label="Previous event"
                    className="p-1.5 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800 disabled:opacity-30 disabled:cursor-not-allowed"
                    title="Previous event (Up arrow)"
                  >
                    <ChevronLeft className="h-5 w-5" />
                  </button>
                  {/* Step Next */}
                  <button
                    disabled={selectedIndex >= filteredEvents.length - 1}
                    onClick={() => setSelectedIndex((prev) => Math.min(prev + 1, filteredEvents.length - 1))}
                    aria-label="Next event"
                    className="p-1.5 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800 disabled:opacity-30 disabled:cursor-not-allowed"
                    title="Next event (Down arrow)"
                  >
                    <ChevronRight className="h-5 w-5" />
                  </button>
                  <button
                    onClick={() => setSelectedEvent(null)}
                    aria-label="Close inspector"
                    className="p-1.5 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800 ml-2"
                  >
                    <X className="h-5 w-5" />
                  </button>
                </div>
              </div>

              {/* Related Activity Banner */}
              {relatedCount > 0 && (
                <div className="mt-4 p-3 rounded-xl bg-indigo-600/10 border border-indigo-500/20 flex items-center justify-between text-xs">
                  <span className="text-indigo-300">
                    <strong>{relatedCount}</strong> other telemetry event{relatedCount > 1 ? "s" : ""} recorded from this user/IP in the fleet.
                  </span>
                  <button
                    onClick={() => {
                      setSearchTerm(selectedEvent.data?.src_ip || selectedEvent.username || "");
                      setSelectedEvent(null);
                    }}
                    className="text-indigo-400 hover:underline inline-flex items-center gap-1 font-medium"
                  >
                    Filter on these <ExternalLink className="h-3 w-3" />
                  </button>
                </div>
              )}

              {/* Quick Field-Level Inspection & Per-field copy */}
              <div className="my-5">
                <span className="text-[11px] font-medium text-slate-400 uppercase tracking-wider block mb-2">
                  Key Attributes (Hover to Copy or Filter)
                </span>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
                  <FieldCopyPill label="Event ID" value={selectedEvent.event_id} />
                  <FieldCopyPill label="Time (UTC)" value={selectedEvent.time} />
                  <FieldCopyPill label="Hostname" value={selectedEvent.hostname} onFilterSelect={(val) => setSearchTerm(val)} />
                  <FieldCopyPill label="Target User" value={selectedEvent.username} onFilterSelect={(val) => setSearchTerm(val)} />
                  <FieldCopyPill label="Service" value={selectedEvent.data?.service} onFilterSelect={(val) => setFilterService(val)} />
                  <FieldCopyPill label="Source IP" value={selectedEvent.data?.src_ip} onFilterSelect={(val) => setSearchTerm(val)} />
                  <FieldCopyPill label="Failure Reason" value={selectedEvent.data?.reason} onFilterSelect={(val) => setFilterReason(val)} />
                  <FieldCopyPill label="Agent ID" value={selectedEvent.agent_id} />
                </div>
              </div>

              {/* Envelope Metadata Summary */}
              <div className="grid grid-cols-2 gap-3 mb-5 text-xs">
                <div className="p-3 rounded-xl bg-slate-950/60 border border-slate-800">
                  <span className="text-slate-500 block mb-1">OCSF Classification</span>
                  <span className="font-mono text-slate-200">Class: {selectedEvent.class_uid} (Auth)</span>
                  <span className="block font-mono text-indigo-400 mt-0.5">Type: {selectedEvent.type_uid}</span>
                </div>
                <div className="p-3 rounded-xl bg-slate-950/60 border border-slate-800">
                  <span className="text-slate-500 block mb-1">Ingest Pipeline</span>
                  <span className="font-mono text-emerald-400 uppercase">{selectedEvent.ingest_source || "websocket"}</span>
                  <span className="block font-mono text-slate-400 mt-0.5">OS: {selectedEvent.host_os || "linux"}</span>
                </div>
              </div>

              {/* Color-Coded Syntax Highlighted JSON */}
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-semibold text-slate-300 uppercase tracking-wider">
                    OCSF Data Envelope (`data` JSONB)
                  </span>
                  <button
                    onClick={handleCopyWholeJson}
                    aria-label="Copy whole JSON payload"
                    className="flex items-center gap-1.5 px-3 py-1 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-lg text-xs transition-colors"
                  >
                    {copiedWhole ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
                    {copiedWhole ? "Copied Envelope" : "Copy All JSON"}
                  </button>
                </div>

                <div className="p-4 rounded-xl bg-slate-950 border border-slate-800 max-h-72 overflow-y-auto">
                  <SyntaxHighlightedJSON jsonString={selectedEvent.data} />
                </div>
              </div>
            </div>

            <div className="pt-4 border-t border-slate-800 flex items-center justify-between">
              <span className="text-xs text-slate-500 font-mono">Press ESC or click backdrop to close</span>
              <button
                onClick={() => setSelectedEvent(null)}
                className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-xl text-xs font-medium transition-colors"
              >
                Close Inspector
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default Events;