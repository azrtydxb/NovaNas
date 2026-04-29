import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { plugins, type DisplayCategory, type PluginIndexEntry } from "../../api/plugins";
import { Icon } from "../../components/Icon";
import { formatBytes } from "../../lib/format";
import { InstallConsent } from "./InstallConsent";

export function Discover() {
  const [cat, setCat] = useState<DisplayCategory | "all">("all");
  const [search, setSearch] = useState("");
  const [picked, setPicked] = useState<PluginIndexEntry | null>(null);

  const cats = useQuery({
    queryKey: ["plugins", "categories"],
    queryFn: () => plugins.listCategories(),
  });
  const list = useQuery({
    queryKey: ["plugins", "index", cat],
    queryFn: () => plugins.listIndex(cat === "all" ? {} : { displayCategory: cat }),
  });

  const filtered = (list.data ?? []).filter((p) => {
    if (!search.trim()) return true;
    const q = search.toLowerCase();
    return (
      p.name.toLowerCase().includes(q) ||
      (p.displayName ?? "").toLowerCase().includes(q) ||
      (p.description ?? "").toLowerCase().includes(q) ||
      (p.tags ?? []).some((t) => t.toLowerCase().includes(q))
    );
  });

  return (
    <div className="discover">
      <aside className="discover__rail">
        <div className="vlist__title">CATEGORIES</div>
        <button className={cat === "all" ? "is-on" : ""} onClick={() => setCat("all")}>
          <Icon name="apps" size={12} />
          <span>All</span>
          <span className="discover__count">
            {(cats.data ?? []).reduce((sum, c) => sum + c.count, 0)}
          </span>
        </button>
        {(cats.data ?? []).map((c) => (
          <button
            key={c.category}
            className={cat === c.category ? "is-on" : ""}
            onClick={() => setCat(c.category)}
          >
            <Icon name={iconForDisplayCategory(c.category)} size={12} />
            <span>{c.displayName}</span>
            <span className="discover__count">{c.count}</span>
          </button>
        ))}
      </aside>
      <main className="discover__body">
        <div className="discover__bar">
          <div className="discover__search">
            <Icon name="search" size={12} />
            <input
              className="discover__search-input"
              placeholder="Search plugins…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          <span className="muted small">
            {filtered.length} {filtered.length === 1 ? "plugin" : "plugins"}
          </span>
        </div>
        {list.isLoading && <div className="discover__msg">Loading plugins…</div>}
        {list.isError && (
          <div className="discover__msg discover__msg--err">
            Failed to load: {(list.error as Error).message}
          </div>
        )}
        {list.data && filtered.length === 0 && !list.isLoading && (
          <div className="discover__msg muted">
            {search ? "No matches." : "No plugins in this category."}
          </div>
        )}
        {filtered.length > 0 && (
          <div className="discover__grid">
            {filtered.map((p) => (
              <PluginCard key={p.name} plugin={p} onInstall={() => setPicked(p)} />
            ))}
          </div>
        )}
      </main>
      {picked && (
        <InstallConsent
          plugin={picked}
          marketplaceId={picked.marketplace}
          onClose={() => setPicked(null)}
        />
      )}
    </div>
  );
}

function PluginCard({
  plugin,
  onInstall,
}: {
  plugin: PluginIndexEntry;
  onInstall: () => void;
}) {
  const v = plugin.versions[0];
  return (
    <div className="mkt-card">
      <div className="mkt-card__head">
        <div className="mkt-card__icon">
          {plugin.name.split("-").slice(-1)[0].slice(0, 2).toUpperCase()}
        </div>
        <div className="mkt-card__id">
          <div className="mkt-card__name">{plugin.displayName ?? plugin.name}</div>
          <div className="mkt-card__author muted">
            {plugin.vendor ?? "—"}
            {v && <> · v{v.version}</>}
          </div>
        </div>
        {plugin.marketplace && (
          <span className="trust-badge trust-badge--official">
            <Icon name="shield" size={9} />
            {plugin.marketplace}
          </span>
        )}
      </div>
      {plugin.description && <div className="mkt-card__desc">{plugin.description}</div>}
      <div className="mkt-card__foot">
        {plugin.displayCategory && <span className="pill">{plugin.displayCategory}</span>}
        {v && <span className="muted mono small">{formatBytes(v.size)}</span>}
        <button className="btn btn--sm btn--primary mkt-card__cta" onClick={onInstall}>
          Install
        </button>
      </div>
      {plugin.tags && plugin.tags.length > 0 && (
        <div className="mkt-card__tags">
          {plugin.tags.map((t) => (
            <span key={t} className="tag mono">
              {t}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

function iconForDisplayCategory(c: string) {
  switch (c) {
    case "backup": return "shield" as const;
    case "files": return "files" as const;
    case "multimedia": return "video" as const;
    case "photos": return "image" as const;
    case "productivity": return "doc" as const;
    case "security": return "lock" as const;
    case "communication": return "bell" as const;
    case "home": return "globe" as const;
    case "developer": return "terminal" as const;
    case "network": return "net" as const;
    case "storage": return "storage" as const;
    case "surveillance": return "monitor" as const;
    case "utilities": return "settings" as const;
    case "observability": return "log" as const;
    default: return "package" as const;
  }
}
