import { ImageResponse } from "next/og";

import { logoMarkDataUri } from "@/lib/logo";

export const size = { width: 180, height: 180 };
export const contentType = "image/png";

// iOS は角を自分で丸めるため、正方形のまま塗りつぶす。
export default function AppleIcon() {
  return new ImageResponse(
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "#FFE4EA",
      }}
    >
      {/* eslint-disable-next-line @next/next/no-img-element -- ImageResponse は next/image を描けない */}
      <img src={logoMarkDataUri()} width={132} height={132} alt="" />
    </div>,
    size,
  );
}
