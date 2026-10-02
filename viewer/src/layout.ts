// Layered layout: a box sits above what it depends on, so arrows point down
// toward the code everything else uses.

import ELK from "elkjs/lib/elk.bundled.js";
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

export async function layout(v: View): Promise<Map<string, Placed>> {
  const result = await elk.layout({
    id: "level",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "DOWN",
      "elk.layered.spacing.nodeNodeBetweenLayers": "80",
      "elk.spacing.nodeNode": "36",
      "elk.spacing.componentComponent": "60",
      "elk.layered.nodePlacement.strategy": "BRANDES_KOEPF",
    },
    children: v.nodes.map((n) => ({ id: n.id, width: boxWidth(n.label), height: BOX_HEIGHT })),
    edges: v.edges.map((e) => ({ id: e.id, sources: [e.source], targets: [e.target] })),
  });
  const placed = new Map<string, Placed>();
  for (const c of result.children ?? []) {
    placed.set(c.id, { x: c.x ?? 0, y: c.y ?? 0, width: c.width ?? 190 });
  }
  return placed;
}
