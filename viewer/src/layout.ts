// Layered layout: a box sits above what it depends on, so arrows point down
// toward the code everything else uses. ELK also routes the arrows, around
// boxes and along orthogonal tracks, so a busy level stays readable.

import ELK from "elkjs/lib/elk.bundled.js";
import type { ElkExtendedEdge } from "elkjs/lib/elk-api";
import type { View } from "./model";

const elk = new ELK();

export const BOX_HEIGHT = 92;

export function boxWidth(label: string): number {
  return Math.min(360, Math.max(240, label.length * 7.6 + 56));
}

export interface Placed {
  x: number;
  y: number;
  width: number;
}

export interface Point {
  x: number;
  y: number;
}

export interface Layout {
  nodes: Map<string, Placed>;
  routes: Map<string, Point[]>; // each edge's polyline, start to end
}

export async function layout(v: View): Promise<Layout> {
  const result = await elk.layout({
    id: "level",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "DOWN",
      "elk.edgeRouting": "ORTHOGONAL",
      "elk.layered.spacing.nodeNodeBetweenLayers": "90",
      "elk.layered.spacing.edgeNodeBetweenLayers": "24",
      "elk.layered.spacing.edgeEdgeBetweenLayers": "14",
      "elk.spacing.nodeNode": "40",
      "elk.spacing.edgeEdge": "14",
      "elk.spacing.componentComponent": "60",
      "elk.layered.nodePlacement.strategy": "BRANDES_KOEPF",
    },
    children: v.nodes.map((n) => ({ id: n.id, width: boxWidth(n.label), height: BOX_HEIGHT })),
    edges: v.edges.map((e) => ({ id: e.id, sources: [e.source], targets: [e.target] })),
  });
  const nodes = new Map<string, Placed>();
  for (const c of result.children ?? []) {
    nodes.set(c.id, { x: c.x ?? 0, y: c.y ?? 0, width: c.width ?? 240 });
  }
  const routes = new Map<string, Point[]>();
  // The result's edges carry the routes ELK computed.
  for (const e of (result.edges ?? []) as ElkExtendedEdge[]) {
    const s = e.sections?.[0];
    if (s) routes.set(e.id, [s.startPoint, ...(s.bendPoints ?? []), s.endPoint]);
  }
  return { nodes, routes };
}

// roundedPath draws a polyline with its corners rounded off.
export function roundedPath(points: Point[], radius = 10): string {
  if (points.length === 0) return "";
  let d = `M ${points[0].x} ${points[0].y}`;
  for (let i = 1; i < points.length - 1; i++) {
    const [a, b, c] = [points[i - 1], points[i], points[i + 1]];
    const r = Math.min(radius, dist(a, b) / 2, dist(b, c) / 2);
    const p = toward(b, a, r);
    const q = toward(b, c, r);
    d += ` L ${p.x} ${p.y} Q ${b.x} ${b.y} ${q.x} ${q.y}`;
  }
  const last = points[points.length - 1];
  return `${d} L ${last.x} ${last.y}`;
}

// midpoint is the point halfway along a polyline, for its label.
export function midpoint(points: Point[]): Point {
  const total = points.slice(1).reduce((sum, p, i) => sum + dist(points[i], p), 0);
  let left = total / 2;
  for (let i = 1; i < points.length; i++) {
    const d = dist(points[i - 1], points[i]);
    if (left <= d) return toward(points[i - 1], points[i], left);
    left -= d;
  }
  return points[points.length - 1];
}

function dist(a: Point, b: Point): number {
  return Math.hypot(b.x - a.x, b.y - a.y);
}

function toward(from: Point, to: Point, length: number): Point {
  const d = dist(from, to) || 1;
  return { x: from.x + ((to.x - from.x) * length) / d, y: from.y + ((to.y - from.y) * length) / d };
}
