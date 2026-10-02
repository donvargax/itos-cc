import { BaseEdge, EdgeLabelRenderer, getBezierPath, type Edge, type EdgeProps } from "@xyflow/react";
import type { ViewEdge } from "./model";
import { midpoint, roundedPath, type Point } from "./layout";

export type RoutedEdgeType = Edge<{ view: ViewEdge; points?: Point[] }, "routed">;

// RoutedEdge follows the route the layout computed, falling back to a curve
// between the boxes before a route exists.
export function RoutedEdge(props: EdgeProps<RoutedEdgeType>) {
  const { data, markerEnd, style, selected } = props;
  const points = data?.points;
  let path: string;
  let label: Point;
  if (points && points.length >= 2) {
    path = roundedPath(points);
    label = midpoint(points);
  } else {
    const [d, x, y] = getBezierPath(props);
    path = d;
    label = { x, y };
  }
  const view = data?.view;
  const text = view?.kind === "http" ? (view.count > 1 ? `HTTP ×${view.count}` : "HTTP") : view && view.count > 1 ? String(view.count) : "";
  return (
    <>
      <BaseEdge path={path} markerEnd={markerEnd} style={style} interactionWidth={14} />
      {text && (
        <EdgeLabelRenderer>
          <div
            className={`edge-label ${view?.kind ?? ""} ${selected ? "selected" : ""}`}
            style={{ transform: `translate(-50%, -50%) translate(${label.x}px, ${label.y}px)` }}
          >
            {text}
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  );
}
