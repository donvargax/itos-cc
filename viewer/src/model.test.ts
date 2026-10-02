import { describe, expect, it } from "vitest";
import type { Graph, GraphNode } from "./api";
import { buildIndex, gradeHue, nearestExisting, startingLevel, SYSTEM, trail, view } from "./model";

const node = (id: string, kind: GraphNode["kind"], extra: Partial<GraphNode> = {}): GraphNode => ({
  id,
  kind,
  label: id.split("/").pop()!,
  parent: id.includes("/") ? id.slice(0, id.lastIndexOf("/")) : undefined,
  ...extra,
});

// shop: api (Go package with a subpackage), web/src/{app,cart,format} with a
// cycle between cart and format, and a deep Kotlin chain.
const graph: Graph = {
  version: 1,
  nodes: [
    node("shop", "repo"),
    node("shop/api", "module", { units: [{ name: "main" } as never] }),
    node("shop/api/store", "module"),
    node("shop/web", "dir"),
    node("shop/web/src", "dir"),
    node("shop/web/src/app", "module"),
    node("shop/web/src/cart", "module"),
    node("shop/web/src/format", "module"),
    node("shop/app", "dir"),
    node("shop/app/src", "dir"),
    node("shop/app/src/main", "dir"),
    node("shop/app/src/main/com", "dir"),
    node("shop/app/src/main/com/acme", "dir"),
    node("shop/app/src/main/com/acme/Invoice", "module"),
    node("shop/app/src/main/com/acme/Money", "module"),
  ],
  edges: [
    { from: "shop/web/src/app", to: "shop/web/src/cart", count: 2 },
    { from: "shop/web/src/cart", to: "shop/web/src/format", count: 1 },
    { from: "shop/web/src/format", to: "shop/web/src/cart", count: 1 },
    { from: "shop/web/src/app", to: "shop/api/store", count: 1 },
    { from: "shop/api", to: "shop/api/store", count: 3 },
    { from: "shop/app/src/main/com/acme/Invoice", to: "shop/app/src/main/com/acme/Money", count: 1 },
  ],
};

const idx = buildIndex(graph);
const labels = (container: string) => view(idx, container).nodes.map((n) => n.label);

describe("view", () => {
  it("collapses single-child directory chains into one box", () => {
    expect(labels("shop")).toEqual(["api", "app/src/main/com/acme", "web/src"]);
    const deep = view(idx, "shop").nodes.find((n) => n.label.startsWith("app"))!;
    expect(deep.target).toBe("shop/app/src/main/com/acme");
    expect(deep.drillable).toBe(true);
  });

  it("rolls module dependencies up to the boxes that contain them", () => {
    const edges = view(idx, "shop").edges.map((e) => `${e.source}->${e.target} ${e.count}`);
    expect(edges).toEqual(["shop/web/src->shop/api 1"]);
  });

  it("shows a package's own code beside its subpackages", () => {
    const v = view(idx, "shop/api");
    expect(v.nodes.map((n) => [n.id, n.self])).toEqual([
      ["shop/api#self", true],
      ["shop/api/store", false],
    ]);
    expect(v.edges.map((e) => `${e.source}->${e.target} ${e.count}`)).toEqual(["shop/api#self->shop/api/store 3"]);
  });

  it("counts dependencies that cross the level's boundary", () => {
    const v = view(idx, "shop/api");
    expect(v.nodes.find((n) => n.id === "shop/api/store")!.incomingOutside).toBe(1);
  });

  it("marks cycles on arrows and boxes", () => {
    const v = view(idx, "shop/web/src");
    const cyclic = v.edges.filter((e) => e.cycle).map((e) => e.id);
    expect(cyclic.sort()).toEqual(["shop/web/src/cart->shop/web/src/format", "shop/web/src/format->shop/web/src/cart"]);
    expect(v.nodes.filter((n) => n.inCycle).map((n) => n.label)).toEqual(["cart", "format"]);
    expect(v.edges.find((e) => e.source.endsWith("app"))!.cycle).toBe(false);
  });

  it("lists repositories at the system level", () => {
    expect(labels(SYSTEM)).toEqual(["shop"]);
  });
});

describe("navigation", () => {
  it("opens a lone repository directly", () => {
    expect(startingLevel(idx)).toBe("shop");
  });

  it("builds breadcrumbs from the repository down", () => {
    expect(trail(idx, "shop/web/src").map((n) => n.label)).toEqual(["shop", "web", "src"]);
  });

  it("falls back to the nearest ancestor that still exists", () => {
    expect(nearestExisting(idx, "shop/web/src/gone/deeper")).toBe("shop/web/src");
    expect(nearestExisting(idx, "elsewhere/x")).toBe(SYSTEM);
  });
});

describe("gradeHue", () => {
  it.each([
    [1, 0],
    [10, 130],
    [5.5, 65],
  ])("grade %d is hue %d", (grade, hue) => {
    expect(gradeHue(grade)).toBe(hue);
  });

  it("has no color without a grade", () => {
    expect(gradeHue(undefined)).toBeNull();
  });
});
