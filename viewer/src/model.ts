// What the canvas shows at one level of the tree: the boxes inside a
// container and the dependencies between them, rolled up from the module
// edges the API reports. Pure functions, so the rules are testable without a
// browser.

import type { Graph, GraphEdge, GraphNode, Kind } from "./api";

// SYSTEM is the level above the repositories.
export const SYSTEM = "";

export interface Index {
  nodes: Map<string, GraphNode>;
  children: Map<string, GraphNode[]>;
  edges: GraphEdge[];
}

export function buildIndex(g: Graph): Index {
  const nodes = new Map(g.nodes.map((n) => [n.id, n]));
  const children = new Map<string, GraphNode[]>();
  for (const n of g.nodes) {
    const parent = n.kind === "repo" ? SYSTEM : (n.parent ?? SYSTEM);
    const list = children.get(parent) ?? [];
    list.push(n);
    children.set(parent, list);
  }
  for (const list of children.values()) list.sort((a, b) => a.label.localeCompare(b.label));
  return { nodes, children, edges: g.edges };
}

export interface ViewNode {
  id: string; // unique on this level
  target: string; // the graph node it stands for
  label: string;
  kind: Kind;
  node: GraphNode;
  // self is the module part of a package that also holds subpackages: its
  // own functions, shown beside its children.
  self: boolean;
  drillable: boolean;
  inCycle: boolean;
  incomingOutside: number; // dependencies from outside this level
  outgoingOutside: number;
}

export interface ViewEdge {
  id: string;
  source: string;
  target: string;
  count: number;
  pairs: [string, string][]; // the module dependencies behind this arrow
  cycle: boolean;
}

export interface View {
  container: string;
  nodes: ViewNode[];
  edges: ViewEdge[];
}

const kids = (idx: Index, id: string) => idx.children.get(id) ?? [];

// collapse follows a directory that holds exactly one thing down to that
// thing, so src/main/kotlin/com/acme is one box rather than five clicks.
function collapse(idx: Index, n: GraphNode): { node: GraphNode; label: string } {
  const labels = [n.label];
  while (n.kind === "dir" && kids(idx, n.id).length === 1) {
    n = kids(idx, n.id)[0];
    labels.push(n.label);
  }
  return { node: n, label: labels.join("/") };
}

export function view(idx: Index, container: string): View {
  const nodes: ViewNode[] = [];
  const make = (node: GraphNode, label: string, self: boolean): ViewNode => ({
    id: self ? `${node.id}#self` : node.id,
    target: node.id,
    label,
    kind: node.kind,
    node,
    self,
    drillable: !self && kids(idx, node.id).length > 0,
    inCycle: false,
    incomingOutside: 0,
    outgoingOutside: 0,
  });
  const here = idx.nodes.get(container);
  if (here?.kind === "module" && (here.units?.length ?? 0) > 0) {
    nodes.push(make(here, here.label, true));
  }
  for (const child of kids(idx, container)) {
    const { node, label } = collapse(idx, child);
    nodes.push(make(node, label, false));
  }

  // Every module maps to the box that contains it on this level.
  const box = new Map<string, string>();
  const claim = (id: string, owner: string) => {
    box.set(id, owner);
    for (const c of kids(idx, id)) claim(c.id, owner);
  };
  for (const v of nodes) {
    if (v.self) box.set(v.target, v.id);
    else claim(v.target, v.id);
  }

  const byId = new Map(nodes.map((v) => [v.id, v]));
  const edges = new Map<string, ViewEdge>();
  for (const e of idx.edges) {
    const a = box.get(e.from);
    const b = box.get(e.to);
    if (a && b && a !== b) {
      const key = `${a}->${b}`;
      const edge = edges.get(key) ?? { id: key, source: a, target: b, count: 0, pairs: [], cycle: false };
      edge.count += e.count;
      edge.pairs.push([e.from, e.to]);
      edges.set(key, edge);
    } else if (a && !b) {
      byId.get(a)!.outgoingOutside += e.count;
    } else if (b && !a) {
      byId.get(b)!.incomingOutside += e.count;
    }
  }
  const list = [...edges.values()];
  markCycles(nodes, list);
  return { container, nodes, edges: list };
}

// markCycles flags every arrow and box inside a strongly connected
// component: dependencies that lead back to where they started.
export function markCycles(nodes: ViewNode[], edges: ViewEdge[]): void {
  const out = new Map<string, string[]>();
  for (const e of edges) out.set(e.source, [...(out.get(e.source) ?? []), e.target]);
  const order = new Map<string, number>();
  const low = new Map<string, number>();
  const stack: string[] = [];
  const onStack = new Set<string>();
  const component = new Map<string, number>();
  let counter = 0;
  let components = 0;
  const sizes: number[] = [];

  const visit = (v: string) => {
    order.set(v, counter);
    low.set(v, counter);
    counter++;
    stack.push(v);
    onStack.add(v);
    for (const w of out.get(v) ?? []) {
      if (!order.has(w)) {
        visit(w);
        low.set(v, Math.min(low.get(v)!, low.get(w)!));
      } else if (onStack.has(w)) {
        low.set(v, Math.min(low.get(v)!, order.get(w)!));
      }
    }
    if (low.get(v) === order.get(v)) {
      let size = 0;
      let w: string;
      do {
        w = stack.pop()!;
        onStack.delete(w);
        component.set(w, components);
        size++;
      } while (w !== v);
      sizes.push(size);
      components++;
    }
  };
  for (const n of nodes) if (!order.has(n.id)) visit(n.id);

  const cyclic = (id: string) => sizes[component.get(id)!] > 1;
  for (const n of nodes) n.inCycle = cyclic(n.id);
  for (const e of edges) e.cycle = component.get(e.source) === component.get(e.target) && cyclic(e.source);
}

// trail is the path from the top to container, for breadcrumbs.
export function trail(idx: Index, container: string): GraphNode[] {
  const out: GraphNode[] = [];
  for (let n = idx.nodes.get(container); n; n = n.parent ? idx.nodes.get(n.parent) : undefined) {
    out.unshift(n);
  }
  return out;
}

// nearestExisting is container, or its closest ancestor still in the graph
// after a rebuild removed it.
export function nearestExisting(idx: Index, container: string): string {
  let id = container;
  while (id !== SYSTEM && !idx.nodes.has(id)) {
    const cut = id.lastIndexOf("/");
    id = cut < 0 ? SYSTEM : id.slice(0, cut);
  }
  return id;
}

// startingLevel opens the only repository directly; several start at the
// system level.
export function startingLevel(idx: Index): string {
  const repos = kids(idx, SYSTEM);
  return repos.length === 1 ? repos[0].id : SYSTEM;
}

// grade colors run from red (1) through amber to green (10).
export function gradeHue(grade: number | undefined): number | null {
  if (grade === undefined) return null;
  return Math.round(((Math.min(10, Math.max(1, grade)) - 1) / 9) * 130);
}
