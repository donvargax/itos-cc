import { useEffect, useMemo, useState } from "react";
import { RiskBar } from "./BoxNode";
import { fetchSource, repoOf, type Unit } from "./api";
import type { Index, ViewEdge, ViewNode } from "./model";

export type Selection = { kind: "node"; node: ViewNode } | { kind: "edge"; edge: ViewEdge } | null;

// unitsUnder lists the functions of a node and everything inside it, worst
// CRAP first.
function unitsUnder(idx: Index, id: string, self: boolean): Unit[] {
  const out: Unit[] = [];
  const walk = (nid: string) => {
    out.push(...(idx.nodes.get(nid)?.units ?? []));
    if (!self) for (const c of idx.children.get(nid) ?? []) walk(c.id);
  };
  walk(id);
  return out.sort((a, b) => (b.crap ?? -1) - (a.crap ?? -1) || b.complexity - a.complexity);
}

const pct = (v: number | null) => (v === null ? "–" : `${v.toFixed(0)}%`);
const num = (v: number | null) => (v === null ? "–" : v.toFixed(1));

export function SidePanel({ idx, selection }: { idx: Index; selection: Selection }) {
  const [unit, setUnit] = useState<Unit | null>(null);
  useEffect(() => setUnit(null), [selection]);

  if (!selection) {
    return (
      <aside className="panel">
        <p className="hint">Click a box for its functions, an arrow for the imports behind it. Double-click a box to open it; Esc goes back up.</p>
      </aside>
    );
  }
  if (selection.kind === "edge") {
    const e = selection.edge;
    return (
      <aside className="panel">
        <h2>
          {short(e.source)} → {short(e.target)}
        </h2>
        <p className="meta">
          {e.kind === "http" ? `${e.count} HTTP request${e.count === 1 ? "" : "s"}` : `${e.count} import${e.count === 1 ? "" : "s"}`}
          {e.cycle && <span className="badge cycle">cycle</span>}
        </p>
        {e.via.length > 0 && (
          <ul className="routes">
            {e.via.map((v) => (
              <li key={v}>
                <code>{v}</code>
              </li>
            ))}
          </ul>
        )}
        <ul className="pairs">
          {e.pairs.map(([from, to]) => (
            <li key={from + to}>
              <code>{short(from)}</code> → <code>{short(to)}</code>
            </li>
          ))}
        </ul>
      </aside>
    );
  }
  return <NodePanel idx={idx} node={selection.node} unit={unit} onUnit={setUnit} />;
}

function NodePanel({ idx, node, unit, onUnit }: { idx: Index; node: ViewNode; unit: Unit | null; onUnit: (u: Unit) => void }) {
  const units = useMemo(() => unitsUnder(idx, node.target, node.self), [idx, node]);
  const m = node.node.metrics;
  const external = node.node.external ?? [];
  return (
    <aside className="panel">
      <h2>{node.self ? `${node.label} (own code)` : node.label}</h2>
      <p className="meta">
        {node.kind}
        {node.node.language && ` · ${node.node.language}`} · <code>{node.target}</code>
      </p>
      {m && (
        <dl className="facts">
          <dt>functions</dt>
          <dd>{m.functions}</dd>
          <dt>worst CRAP</dt>
          <dd>{m.max_crap?.toFixed(1) ?? "–"}</dd>
          <dt>by CRAP</dt>
          <dd>
            {m.crap_bands.high} risky · {m.crap_bands.medium} worth a look · {m.crap_bands.low} low · {m.crap_bands.unknown} unmeasured
          </dd>
          <dt>mutants</dt>
          <dd>
            {m.killed} killed · {m.survived} survived · {m.uncovered} uncovered
          </dd>
          <dt>mutation-tested</dt>
          <dd>
            {m.mutated} of {m.functions} functions
          </dd>
          {m.stale > 0 && (
            <>
              <dt>stale</dt>
              <dd>{m.stale} functions changed since mutation testing</dd>
            </>
          )}
          <dt>imports from outside</dt>
          <dd>
            {node.incomingOutside} in · {node.outgoingOutside} out
          </dd>
        </dl>
      )}
      {m && m.functions > 0 && <RiskBar bands={m.crap_bands} />}
      {(node.node.serves?.length ?? 0) > 0 && <Routes title="serves" routes={node.node.serves!} />}
      {(node.node.calls?.length ?? 0) > 0 && <Routes title="calls" routes={node.node.calls!} />}
      {external.length > 0 && (
        <p className="external">
          uses {external.map((x) => <code key={x}>{x}</code>)}
        </p>
      )}
      {units.length > 0 && (
        <table className="units">
          <thead>
            <tr>
              <th>function</th>
              <th title="cyclomatic complexity">CC</th>
              <th>cov</th>
              <th>CRAP</th>
              <th title="killed / survived / uncovered mutants">mut</th>
            </tr>
          </thead>
          <tbody>
            {units.slice(0, 200).map((u) => (
              <tr key={u.file + u.line} onClick={() => onUnit(u)} className={unit === u ? "active" : ""}>
                <td title={`${u.file}:${u.line}`}>
                  {u.name}
                  {u.duplicates > 0 && <span className="badge muted">dup</span>}
                </td>
                <td>{u.complexity}</td>
                <td>{pct(u.coverage)}</td>
                <td className={u.crap !== null && u.crap >= 30 ? "bad" : ""}>{num(u.crap)}</td>
                <td className={u.survived > 0 ? "bad" : ""}>
                  {u.mutated ? `${u.killed}/${u.survived}/${u.uncovered}` : "–"}
                  {u.stale && <span className="badge muted">stale</span>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {unit && <SourceView repo={repoOf(node.target)} unit={unit} />}
    </aside>
  );
}

function Routes({ title, routes }: { title: string; routes: string[] }) {
  return (
    <div className="routes">
      <span className="meta">{title}</span>
      {routes.map((r) => (
        <code key={r}>{r}</code>
      ))}
    </div>
  );
}

function SourceView({ repo, unit }: { repo: string; unit: Unit }) {
  const [text, setText] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    setText(null);
    setError(null);
    fetchSource(repo, unit.file).then(setText, (e) => setError(String(e)));
  }, [repo, unit.file]);
  if (error) return <p className="error">{error}</p>;
  if (text === null) return <p className="hint">loading {unit.file}…</p>;
  const lines = text.split("\n");
  const from = Math.max(1, unit.line - 3);
  const to = Math.min(lines.length, unit.end_line + 3);
  return (
    <div className="source-view">
      <div className="source-view-file">
        {unit.file}:{unit.line}
      </div>
      <pre>
        {lines.slice(from - 1, to).map((l, i) => {
          const n = from + i;
          const inside = n >= unit.line && n <= unit.end_line;
          return (
            <div key={n} className={inside ? "in" : "out"}>
              <span className="ln">{n}</span>
              {l || " "}
            </div>
          );
        })}
      </pre>
    </div>
  );
}

function short(id: string): string {
  return id.replace(/#self$/, "").split("/").slice(1).join("/") || id;
}
