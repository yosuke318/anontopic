import Link from "next/link";

import { siteName } from "@/lib/site";

export function SiteHeader() {
  return (
    <header className="border-line bg-background/90 sticky top-0 z-10 border-b backdrop-blur">
      <div className="mx-auto flex h-14 w-full max-w-5xl items-center justify-between px-5">
        <Link href="/" className="text-lg font-bold tracking-tight">
          {siteName}
        </Link>
        <nav aria-label="サイト内" className="flex items-center gap-5 text-sm">
          <Link href="/about" className="text-muted hover:text-foreground transition-colors">
            サービスについて
          </Link>
          <Link
            href="/topics"
            className="bg-brand text-brand-contrast hover:bg-brand-hover rounded-full px-4 py-2 font-medium transition-colors"
          >
            はじめる
          </Link>
        </nav>
      </div>
    </header>
  );
}
