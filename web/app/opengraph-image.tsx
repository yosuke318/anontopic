import { ImageResponse } from "next/og";

import { siteName } from "@/lib/site";

export const alt = "anontopic - 話したいことから始まる匿名チャット";
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

// ImageResponse に同梱されているフォントは日本語の字形を持たないため、
// 画像には英字だけを置き、日本語の説明は og:title と og:description で渡す。
export default function OpengraphImage() {
  return new ImageResponse(
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        flexDirection: "column",
        justifyContent: "space-between",
        padding: 80,
        background: "#0e1013",
        color: "#eceef1",
      }}
    >
      <div style={{ display: "flex", gap: 20 }}>
        {["#5eddcd", "#2f7f76", "#1e232a"].map((color) => (
          <div key={color} style={{ width: 56, height: 56, borderRadius: 28, background: color }} />
        ))}
      </div>
      <div style={{ display: "flex", flexDirection: "column" }}>
        <div style={{ fontSize: 108, fontWeight: 700, letterSpacing: -3 }}>{siteName}</div>
        <div style={{ marginTop: 24, fontSize: 40, color: "#9aa4b2" }}>
          anonymous text chat, one topic at a time
        </div>
      </div>
    </div>,
    size,
  );
}
