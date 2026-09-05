import Link from "next/link";

import { siteName } from "@/lib/site";

const purpose = `${siteName} は、雑談・趣味・相談のための匿名テキストチャットです。出会いや交際を目的とした利用はできません。`;

export function SiteFooter() {
  return (
    <footer className="border-line mt-auto border-t">
      <div className="text-muted mx-auto flex w-full max-w-5xl flex-col gap-4 px-5 py-8 text-sm sm:flex-row sm:items-center sm:justify-between">
        <p>{purpose}</p>
        <nav aria-label="フッター" className="flex gap-5">
          <Link href="/about" className="hover:text-foreground transition-colors">
            サービスについて
          </Link>
          <Link href="/topics" className="hover:text-foreground transition-colors">
            トピックを選ぶ
          </Link>
        </nav>
      </div>
    </footer>
  );
}
