import { ImageResponse } from "next/og";

import { logoMarkDataUri } from "@/lib/logo";
import { siteName, siteTagline, siteUrl } from "@/lib/site";

export const alt = `${siteName} - ${siteTagline}`;
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

const colors = {
  ground: "#FFF8F4",
  ink: "#3B2F36",
  muted: "#74636C",
  blobPink: "#FFE4EA",
  blobMint: "#DDF3EA",
};

// ImageResponse に同梱されているフォントは日本語の字形を持たない。画像に出す文字だけを含む
// フォントを、ビルドのときに Google Fonts から取り寄せる。User-Agent を付けない要求には
// TrueType で返ってくるため、そのまま ImageResponse に渡せる。
async function loadGoogleFont(family: string, weight: number, text: string): Promise<ArrayBuffer> {
  const params = new URLSearchParams({ family: `${family}:wght@${weight}`, text });
  const css = await (await fetch(`https://fonts.googleapis.com/css2?${params}`)).text();
  const source = css.match(/src: url\((.+?)\) format\('(?:opentype|truetype)'\)/);
  if (!source) {
    throw new Error(`no font file in the Google Fonts response for ${family} ${weight}`);
  }
  const response = await fetch(source[1]);
  if (!response.ok) {
    throw new Error(`fetching the font file for ${family} ${weight} failed: ${response.status}`);
  }
  return response.arrayBuffer();
}

export default async function OpengraphImage() {
  const host = siteUrl.host;
  const font = await loadGoogleFont("Zen Maru Gothic", 900, `${siteName}${siteTagline}${host}`);

  return new ImageResponse(
    <div
      style={{
        position: "relative",
        width: "100%",
        height: "100%",
        display: "flex",
        flexDirection: "column",
        justifyContent: "space-between",
        padding: "78px 84px",
        background: colors.ground,
        color: colors.ink,
        fontFamily: "Zen Maru Gothic",
        overflow: "hidden",
      }}
    >
      <div
        style={{
          position: "absolute",
          top: -96,
          right: -120,
          width: 624,
          height: 624,
          borderRadius: "46% 54% 60% 40% / 52% 44% 56% 48%",
          background: colors.blobPink,
        }}
      />
      <div
        style={{
          position: "absolute",
          bottom: 72,
          right: 408,
          width: 156,
          height: 156,
          borderRadius: 78,
          background: colors.blobMint,
        }}
      />
      <div style={{ display: "flex", alignItems: "center", gap: 36 }}>
        {/* eslint-disable-next-line @next/next/no-img-element -- ImageResponse は next/image を描けない */}
        <img src={logoMarkDataUri()} width={168} height={168} alt="" />
        <div style={{ fontSize: 138, lineHeight: 1, letterSpacing: 4 }}>{siteName}</div>
      </div>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-end" }}>
        <div style={{ fontSize: 46 }}>{siteTagline}</div>
        <div style={{ fontSize: 28, color: colors.muted }}>{host}</div>
      </div>
    </div>,
    {
      ...size,
      fonts: [{ name: "Zen Maru Gothic", data: font, style: "normal", weight: 900 }],
    },
  );
}
