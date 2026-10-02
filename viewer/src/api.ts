// The HTTP API `itos-cc serve` publishes. These types mirror its JSON.

export type Kind = "repo" | "dir" | "module";

export interface Unit {
  name: string;
  namespace: string;
  file: string;
  line: number;
  end_line: number;
  complexity: number;
  coverage: number | null;
  crap: number | null;
  killed: number;
  survived: number;
  uncovered: number;
  mutated: boolean;
  stale: boolean;
  duplicates: number;
}

// Bands count functions by CRAP: low ≤5, medium <30, high ≥30, unknown
// without coverage.
export interface Bands {
  low: number;
  medium: number;
  high: number;
  unknown: number;
}

export interface Metrics {
  functions: number;
  mutated: number; // functions with mutation results
  crap_bands: Bands;
  max_crap?: number;
  killed: number;
  survived: number;
  uncovered: number;
  stale: number;
  duplicates: number;
  crap_grade?: number;
  mutation_grade?: number;
  grade?: number;
}

export interface GraphNode {
  id: string;
  parent?: string;
  kind: Kind;
  label: string;
  language?: string;
  files?: string[];
  units?: Unit[];
  external?: string[];
  serves?: string[]; // HTTP routes the module handles
  calls?: string[]; // HTTP requests the module makes
  metrics?: Metrics;
}

// An import edge is a source dependency; an http edge is a request that
// matches a route the target serves, listed in via.
export interface GraphEdge {
  from: string;
  to: string;
  count: number;
  kind: "import" | "http";
  via?: string[];
}

export interface Graph {
  version: number;
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export async function fetchGraph(): Promise<Graph> {
  const res = await fetch("/api/graph");
  if (!res.ok) throw new Error(`graph: ${res.status}`);
  return res.json();
}

export async function fetchSource(repo: string, file: string): Promise<string> {
  const res = await fetch(`/api/source?repo=${encodeURIComponent(repo)}&file=${encodeURIComponent(file)}`);
  if (!res.ok) throw new Error(`source: ${res.status}`);
  return res.text();
}

// subscribe calls onVersion with each graph version the server announces,
// and onStatus as the connection drops and recovers. EventSource reconnects
// by itself.
export function subscribe(onVersion: (v: number) => void, onStatus: (live: boolean) => void): () => void {
  const events = new EventSource("/api/events");
  events.addEventListener("version", (e) => onVersion(Number((e as MessageEvent).data)));
  events.onopen = () => onStatus(true);
  events.onerror = () => onStatus(false);
  return () => events.close();
}

// repoOf is the repository a node id belongs to: its first segment.
export function repoOf(id: string): string {
  return id.split("/")[0];
}
