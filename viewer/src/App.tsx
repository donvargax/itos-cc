import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Background,
  Controls,
  MarkerType,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  type Edge,
} from "@xyflow/react";
import { fetchGraph, subscribe, type Graph } from "./api";
import { buildIndex, nearestExisting, startingLevel, SYSTEM, trail, view, type ViewEdge } from "./model";
import { BOX_HEIGHT, layout, type Placed } from "./layout";
import { BoxNode, type BoxNodeType } from "./BoxNode";
import { SidePanel, type Selection } from "./SidePanel";

const nodeTypes = { box: BoxNode };

// changedNodes are the ids whose numbers or functions differ between two
// graphs, so a live update can point at what moved.
function changedNodes(before: Graph | null, after: Graph): Set<string> {
  if (!before) return new Set();
  const old = new Map(before.nodes.map((n) => [n.id, JSON.stringify([n.metrics, n.units])]));
  return new Set(after.nodes.filter((n) => old.get(n.id) !== JSON.stringify([n.metrics, n.units])).map((n) => n.id));
}

function Canvas() {
  const [graph, setGraph] = useState<Graph | null>(null);
  const [container, setContainer] = useState<string | null>(null);
  const [selection, setSelection] = useState<Selection>(null);
  const [live, setLive] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [changed, setChanged] = useState<Set<string>>(new Set());
  const [placed, setPlaced] = useState<Map<string, Placed>>(new Map());
  const graphRef = useRef<Graph | null>(null);
  // Boxes call the latest open() without being rebuilt when it changes.
  const openRef = useRef<(id: string) => void>(() => {});
  const flow = useReactFlow();

  const load = useCallback(async () => {
    try {
      const next = await fetchGraph();
      const diff = changedNodes(graphRef.current, next);
      graphRef.current = next;
      setGraph(next);
      setError(null);
      if (diff.size > 0) {
        setChanged(diff);
        setTimeout(() => setChanged(new Set()), 2500);
      }
    } catch (e) {
      setError(String(e));
    }
  }, []);

  useEffect(() => {
    load();
    return subscribe((v) => {
      if (!graphRef.current || v > graphRef.current.version) load();
    }, setLive);
  }, [load]);

  const idx = useMemo(() => (graph ? buildIndex(graph) : null), [graph]);
  const level = idx && container !== null ? nearestExisting(idx, container) : null;
  useEffect(() => {
    if (idx && container === null) setContainer(startingLevel(idx));
  }, [idx, container]);
  const current = useMemo(() => (idx && level !== null ? view(idx, level) : null), [idx, level]);

  useEffect(() => {
    if (!current) return;
    let cancelled = false;
    layout(current).then((p) => !cancelled && setPlaced(p));
    return () => {
      cancelled = true;
    };
  }, [current]);

  // Fit the view when the level changes, not on every live update.
  useEffect(() => {
    const t = setTimeout(() => flow.fitView({ padding: 0.15, duration: 250 }), 60);
    return () => clearTimeout(t);
  }, [level, flow]);

  const nodes: BoxNodeType[] = useMemo(
    () =>
      (current?.nodes ?? []).map((v) => {
        const p = placed.get(v.id) ?? { x: 0, y: 0, width: 190 };
        return {
          id: v.id,
          type: "box",
          position: { x: p.x, y: p.y },
          width: p.width,
          height: BOX_HEIGHT,
          data: { view: v, width: p.width, changed: changed.has(v.target), onOpen: openRef.current },
          selected: selection?.kind === "node" && selection.node.id === v.id,
        };
      }),
    [current, placed, changed, selection],
  );

  const edges: Edge[] = useMemo(
    () =>
      (current?.edges ?? []).map((e) => ({
        id: e.id,
        source: e.source,
        target: e.target,
        label: e.count > 1 ? String(e.count) : undefined,
        className: e.cycle ? "edge cycle" : "edge",
        selected: selection?.kind === "edge" && selection.edge.id === e.id,
        markerEnd: { type: MarkerType.ArrowClosed, width: 18, height: 18 },
        style: { strokeWidth: 1.2 + Math.log2(e.count) },
        data: { view: e },
      })),
    [current, selection],
  );

  const open = useCallback(
    (id: string) => {
      if (!current) return;
      const v = current.nodes.find((n) => n.id === id);
      if (v?.drillable) {
        setContainer(v.target);
        setSelection(null);
      }
    },
    [current],
  );

  openRef.current = open;

  const up = useCallback(() => {
    if (!idx || level === null || level === SYSTEM) return;
    const parent = idx.nodes.get(level)?.parent ?? SYSTEM;
    if (parent === SYSTEM && startingLevel(idx) !== SYSTEM) return;
    setContainer(parent);
    setSelection(null);
  }, [idx, level]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") up();
      if (e.key === "Enter" && selection?.kind === "node") open(selection.node.id);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [up, open, selection]);

  if (error && !graph) return <div className="fatal">Cannot reach the API: {error}. Is `itos-cc serve` running?</div>;
  if (!idx || !current || level === null) return <div className="fatal">Loading…</div>;

  const crumbs = trail(idx, level);
  const repos = idx.children.get(SYSTEM)?.length ?? 0;
  return (
    <div className="app">
      <header>
        <nav className="crumbs">
          {repos > 1 && (
            <button onClick={() => setContainer(SYSTEM)} className={level === SYSTEM ? "here" : ""}>
              system
            </button>
          )}
          {crumbs.map((n) => (
            <button key={n.id} onClick={() => setContainer(n.id)} className={n.id === level ? "here" : ""}>
              {n.label}
            </button>
          ))}
        </nav>
        <div className="status">
          <span className={live ? "dot live" : "dot down"} />
          {live ? "live" : "reconnecting"} · v{graph?.version}
        </div>
        <div className="legend">
          <span className="swatch" style={{ "--hue": 0 } as React.CSSProperties} /> risky
          <span className="swatch" style={{ "--hue": 130 } as React.CSSProperties} /> healthy
          <span className="swatch none" /> no data
          <span className="line cycle" /> cycle
        </div>
      </header>
      <main>
        <div className="canvas">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            onNodeClick={(_, n) => setSelection({ kind: "node", node: (n as BoxNodeType).data.view })}
            zoomOnDoubleClick={false}
            onEdgeClick={(_, e) => setSelection({ kind: "edge", edge: e.data!.view as ViewEdge })}
            onPaneClick={() => setSelection(null)}
            nodesConnectable={false}
            nodesDraggable
            minZoom={0.1}
            proOptions={{ hideAttribution: true }}
          >
            <Background gap={24} />
            <Controls showInteractive={false} />
            <MiniMap pannable zoomable nodeColor={(n) => miniColor(n as BoxNodeType)} />
          </ReactFlow>
          {current.nodes.length === 0 && <div className="empty">Nothing here.</div>}
        </div>
        <SidePanel idx={idx} selection={selection} />
      </main>
    </div>
  );
}

function miniColor(n: BoxNodeType): string {
  const g = n.data.view.node.metrics?.grade;
  return g === undefined ? "#9aa0a6" : `hsl(${((g - 1) / 9) * 130} 60% 50%)`;
}

export function App() {
  return (
    <ReactFlowProvider>
      <Canvas />
    </ReactFlowProvider>
  );
}
