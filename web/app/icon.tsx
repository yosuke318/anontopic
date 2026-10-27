import { ImageResponse } from "next/og";

import { logoMarkDataUri } from "@/lib/logo";

// 高精細の画面でもにじまないよう、表示する大きさの 2 倍で作る。
export const size = { width: 64, height: 64 };
export const contentType = "image/png";

export default function Icon() {
  return new ImageResponse(
    // eslint-disable-next-line @next/next/no-img-element -- ImageResponse は next/image を描けない
    <img src={logoMarkDataUri()} width={64} height={64} alt="" />,
    size,
  );
}
