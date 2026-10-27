// ロゴのマーク。手前の吹き出しが自分（ピンク）、奥が会話の相手（ラベンダー）で、
// チャット画面で自分と参加者 B に使う色と同じ。64×64 の座標で描く。
export const logoColors = {
  front: "#FF8FAB",
  back: "#C7B9F2",
  dots: "#3B2F36",
} as const;

export const frontBubblePath =
  "M12 28h26a9 9 0 0 1 9 9v10a9 9 0 0 1-9 9H21l-9 6 2-6h-2a9 9 0 0 1-9-9V37a9 9 0 0 1 9-9z";

export const backBubblePath =
  "M26 6h26a9 9 0 0 1 9 9v10a9 9 0 0 1-9 9h-1l2 6-8-6H26a9 9 0 0 1-9-9V15a9 9 0 0 1 9-9z";

export const dotCenters = [16, 25, 34].map((cx) => ({ cx, cy: 42, r: 2.8 }));

// 奥の吹き出しは、手前の吹き出しの輪郭から 1.5 外側までを切り抜いて描く。
// 下地の色によらず、重なりの境目に隙間が見える。
export const gapStrokeWidth = 3;

// ImageResponse の画像に埋め込むための SVG。
export function logoMarkSvg(): string {
  const dots = dotCenters
    .map(({ cx, cy, r }) => `<circle cx="${cx}" cy="${cy}" r="${r}" fill="${logoColors.dots}"/>`)
    .join("");
  return [
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">',
    '<mask id="gap" maskUnits="userSpaceOnUse" x="0" y="0" width="64" height="64">',
    '<rect width="64" height="64" fill="#fff"/>',
    `<path d="${frontBubblePath}" fill="#000" stroke="#000" stroke-width="${gapStrokeWidth}"/>`,
    "</mask>",
    `<path d="${backBubblePath}" fill="${logoColors.back}" mask="url(#gap)"/>`,
    `<path d="${frontBubblePath}" fill="${logoColors.front}"/>`,
    dots,
    "</svg>",
  ].join("");
}

export function logoMarkDataUri(): string {
  return `data:image/svg+xml;base64,${btoa(logoMarkSvg())}`;
}
