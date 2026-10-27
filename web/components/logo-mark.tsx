import { useId } from "react";

import {
  backBubblePath,
  dotCenters,
  frontBubblePath,
  gapStrokeWidth,
  logoColors,
} from "@/lib/logo";

type Props = {
  className?: string;
};

export function LogoMark({ className }: Props) {
  // 同じページに複数置いても、マスクの ID がぶつからないようにする。
  const maskId = `logo-gap-${useId().replace(/:/g, "")}`;

  return (
    <svg viewBox="0 0 64 64" className={className} aria-hidden="true">
      <mask id={maskId} maskUnits="userSpaceOnUse" x="0" y="0" width="64" height="64">
        <rect width="64" height="64" fill="#fff" />
        <path d={frontBubblePath} fill="#000" stroke="#000" strokeWidth={gapStrokeWidth} />
      </mask>
      <path d={backBubblePath} fill={logoColors.back} mask={`url(#${maskId})`} />
      <path d={frontBubblePath} fill={logoColors.front} />
      {dotCenters.map(({ cx, cy, r }) => (
        <circle key={cx} cx={cx} cy={cy} r={r} fill={logoColors.dots} />
      ))}
    </svg>
  );
}
