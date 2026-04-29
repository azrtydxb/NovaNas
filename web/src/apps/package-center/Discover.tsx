import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { plugins, type DisplayCategory } from "../../api/plugins";
import { formatBytes } from "../../lib/format";

export function Discover() {
  const [cat, setCat] = useState<DisplayCategory | "all">("all");

  const cats = useQuery({
    queryKey: ["plugins", "categories"],
    queryFn: () => plugins.listCategories(),
  });
  const list = useQuery({
    queryKey: ["plugins", "index", cat],
    queryFn: () => plugins.listIndex(cat === "all" ? {} : { displayCategory: cat }),
  });

  return (
    <div className="discover">
      <aside className="discover__rail">
        <div className="vlist__title">CATEGORIES</div>
        <button className={cat === "all" ? "is-on" : ""} onClick={() => setCat("all")}>
          All
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
            {c.displayName}
            <span className="discover__count">{c.count}</span>
          </button>
        ))}
      </aside>
      <main className="discover__body">
        {list.isLoading && <div className="discover__msg">Loading plugins…</div>}
        {list.isError && (
          <div className="discover__msg discover__msg--err">
            Failed to load: {(list.error as Error).message}
          </div>
        )}
        {list.data && list.data.length === 0 && (
          <div className="discover__msg">No plugins in this category.</div>
        )}
        {list.data && list.data.length > 0 && (
          <div className="discover__grid">
            {list.data.map((p) => {
              const v = p.versions[0];
              return (
                <div key={p.name} className="mkt-card">
                  <div className="mkt-card__head">
                    <div className="mkt-card__icon">
                      {p.name.split("-").slice(-1)[0].slice(0, 2).toUpperCase()}
                    </div>
                    <div className="mkt-card__id">
                      <div className="mkt-card__name">{p.displayName ?? p.name}</div>
                      <div className="mkt-card__author">
                        {p.vendor ?? "—"} · v{v?.version ?? "—"}
                      </div>
                    </div>
                    {p.marketplace && (
                      <span className="trust-badge">
                        <span className="dot" /> {p.marketplace}
                      </span>
                    )}
                  </div>
                  {p.description && <div className="mkt-card__desc">{p.description}</div>}
                  <div className="mkt-card__foot">
                    {p.displayCategory && (
                      <span className="pill">{p.displayCategory}</span>
                    )}
                    {v && <span className="muted mono">{formatBytes(v.size)}</span>}
                    <button className="btn btn--sm btn--primary mkt-card__cta">
                      Install
                    </button>
                  </div>
                  {p.tags && p.tags.length > 0 && (
                    <div className="mkt-card__tags">
                      {p.tags.map((t) => (
                        <span key={t} className="tag">
                          {t}
                        </span>
                      ))}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </main>
    </div>
  );
}
