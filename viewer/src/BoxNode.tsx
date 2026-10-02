import { Handle, Position, type Node, type NodeProps } from "@xyflow/react";
import { gradeHue, type ViewNode } from "./model";
import { BOX_HEIGHT } from "./layout";

export type BoxData = { view: ViewNode; width: number; changed: boolean; onOpen: (id: string) => void };
export type BoxNodeType = Node<BoxData, "box">;

const icon = { repo: "◆", dir: "▸", module: "◻" } as const;

export function BoxNode({ data, selected }: NodeProps<BoxNodeType>) {
  const { view, width, changed, onOpen } = data;
  const m = view.node.metrics;
  const hue = gradeHue(m?.grade);
  const style = {
    width,
    height: BOX_HEIGHT,
    "--hue": hue ?? 0,
  } as React.CSSProperties;
  const classes = [
    "box",
    hue === null ? "ungraded" : "graded",
    view.drillable ? "drillable" : "",
    view.inCycle ? "cyclic" : "",
    m?.stale ? "stale" : "",
    selected ? "selected" : "",
    changed ? "changed" : "",
  ].join(" ");
  const mutation = m && m.killed + m.survived > 0 ? Math.round((100 * m.killed) / (m.killed + m.survived)) : null;
  return (
    // Double-click is handled here rather than by the canvas: the first click
    // selects the box, and that re-render would swallow the canvas's event.
    <div className={classes} style={style} title={view.target} onDoubleClick={() => onOpen(view.id)}>
      <Handle type="target" position={Position.Top} className="handle" />
      <div className="box-title">
        <span className="box-icon">{view.self ? "◼" : icon[view.kind]}</span>
        <span className="box-label">{view.self ? `${view.label} (own code)` : view.label}</span>
        {view.inCycle && <span className="badge cycle" title="part of a dependency cycle">cycle</span>}
      </div>
      <div className="box-stats">
        <span>{m?.functions ?? 0} fn</span>
        <span title="worst CRAP score">CRAP {m?.max_crap !== undefined ? m.max_crap.toFixed(1) : "–"}</span>
        <span title="killed / (killed + survived)">mut {mutation !== null ? `${mutation}%` : "–"}</span>
      </div>
      <div className="box-badges">
        {m && m.survived > 0 && <span className="badge warn">{m.survived} survived</span>}
        {m && m.stale > 0 && <span className="badge muted" title="functions changed since mutation testing">{m.stale} stale</span>}
        {m && m.duplicates > 0 && <span className="badge muted">{m.duplicates} dup</span>}
        {view.drillable && <span className="badge open">open ↵</span>}
      </div>
      <Handle type="source" position={Position.Bottom} className="handle" />
    </div>
  );
}
