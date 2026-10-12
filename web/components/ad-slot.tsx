"use client";

import Script from "next/script";
import { useEffect, useRef } from "react";

import { adsenseClient, adsenseSlot, adUnitsEnabled } from "@/lib/ads";

declare global {
  interface Window {
    adsbygoogle?: object[];
  }
}

const label = "広告";

// 開発中は AdSense のテスト配信にし、表示や操作を広告の実績に数えさせない。
const adTest = process.env.NODE_ENV === "production" ? undefined : "on";

function AdUnit({ className }: { className?: string }) {
  const requested = useRef(false);

  // push は ins 1 つにつき 1 回だけ呼ぶ。2 回目は AdSense がエラーにする。
  // スクリプトの読み込み前に積んだ分は、読み込み後にまとめて処理される。
  useEffect(() => {
    if (requested.current) {
      return;
    }
    requested.current = true;
    (window.adsbygoogle = window.adsbygoogle || []).push({});
  }, []);

  // 配信する広告が無かった枠は、見出しと余白ごと畳む。
  return (
    <aside
      aria-label={label}
      className={`${className ?? ""} has-[[data-ad-status=unfilled]]:hidden`}
    >
      <p className="text-muted text-xs">{label}</p>
      <ins
        className="adsbygoogle mt-2 block min-h-[280px]"
        data-ad-client={adsenseClient}
        data-ad-slot={adsenseSlot}
        data-ad-format="auto"
        data-full-width-responsive="true"
        data-adtest={adTest}
      />
      <Script
        id="adsbygoogle"
        src={`https://pagead2.googlesyndication.com/pagead/js/adsbygoogle.js?client=${adsenseClient}`}
        strategy="lazyOnload"
        crossOrigin="anonymous"
      />
    </aside>
  );
}

// AdSlot は手動で置く広告ユニットを描く。AdSense のスクリプトは、この枠を含むページを
// 開いたときに、ページの読み込みが終わってから取得する（ADR-0038）。
export function AdSlot({ className }: { className?: string }) {
  if (!adUnitsEnabled) {
    return null;
  }
  return <AdUnit className={className} />;
}
