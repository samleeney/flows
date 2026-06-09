import { BaseEdge, type EdgeProps } from "@xyflow/react";

const EXIT_ROUTE_SOURCE_RUN = 230;
const EXIT_ROUTE_TARGET_RUN = 56;
const EXIT_ROUTE_LANE_BASE = 96;
const EXIT_ROUTE_LABEL_OFFSET_X = 118;
const EXIT_ROUTE_LABEL_OFFSET_Y = -18;

export function ExitRouteEdge({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  markerEnd,
  style,
  interactionWidth,
  label,
  labelStyle,
  labelBgStyle,
}: EdgeProps) {
  const sourceRunX = sourceX + EXIT_ROUTE_SOURCE_RUN;
  const targetRunX = targetX - EXIT_ROUTE_TARGET_RUN;
  const laneY = targetY - EXIT_ROUTE_LANE_BASE;
  const path = [
    `M ${sourceX} ${sourceY}`,
    `L ${sourceRunX} ${sourceY}`,
    `L ${sourceRunX} ${laneY}`,
    `L ${targetRunX} ${laneY}`,
    `L ${targetRunX} ${targetY}`,
    `L ${targetX} ${targetY}`,
  ].join(" ");

  return (
    <BaseEdge
      id={id}
      path={path}
      markerEnd={markerEnd}
      style={style}
      interactionWidth={interactionWidth}
      label={label}
      labelX={sourceX + EXIT_ROUTE_LABEL_OFFSET_X}
      labelY={sourceY + EXIT_ROUTE_LABEL_OFFSET_Y}
      labelStyle={labelStyle}
      labelBgStyle={labelBgStyle}
      labelBgPadding={[6, 4]}
      labelBgBorderRadius={3}
    />
  );
}
