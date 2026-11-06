import type { Metadata, Viewport } from "next";
import { Zen_Maru_Gothic } from "next/font/google";

import { adsenseClient } from "@/lib/ads";
import { siteDescription, siteName, siteUrl } from "@/lib/site";
import "./globals.css";

// 日本語の字形は Google Fonts が文字の範囲ごとに分けて配るため、ブラウザはページで使う
// 文字の分だけを取りに行く。範囲の一覧はラテン文字に限らず大きいため、先読みはしない。
const zenMaruGothic = Zen_Maru_Gothic({
  weight: ["500", "700", "900"],
  subsets: ["latin"],
  display: "swap",
  preload: false,
  variable: "--font-zen-maru",
});

export const metadata: Metadata = {
  metadataBase: siteUrl,
  title: {
    default: `匿名チャット「${siteName}」- 登録不要で話題を選んでチャット`,
    template: `%s | ${siteName}`,
  },
  description: siteDescription,
  applicationName: siteName,
  keywords: [
    "チャット",
    "匿名チャット",
    "雑談チャット",
    "トピック",
    "テキストチャット",
    "登録不要",
  ],
  openGraph: {
    type: "website",
    locale: "ja_JP",
    siteName,
    title: `匿名チャット「${siteName}」- 登録不要で話題を選んでチャット`,
    description: siteDescription,
    url: "/",
  },
  twitter: {
    card: "summary_large_image",
    title: `匿名チャット「${siteName}」- 登録不要で話題を選んでチャット`,
    description: siteDescription,
  },
  robots: {
    index: true,
    follow: true,
  },
  // AdSense がサイトの所有者を確かめるためのタグ。広告のスクリプトとは別に、全ページに置く。
  ...(adsenseClient !== "" && { other: { "google-adsense-account": adsenseClient } }),
};

export const viewport: Viewport = {
  themeColor: "#fff8f4",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="ja" className={`${zenMaruGothic.variable} h-full antialiased`}>
      <body className="font-sans min-h-full">{children}</body>
    </html>
  );
}
