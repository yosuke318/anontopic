import Link from "next/link";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "ページが見つかりません",
  robots: { index: false, follow: false },
};

export default function NotFound() {
  return (
    <div className="mx-auto w-full max-w-2xl px-5 py-24">
      <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">ページが見つかりません</h1>
      <p className="text-muted mt-6 leading-8">
        お探しのページは移動したか、削除された可能性があります。
      </p>
      <Link
        href="/"
        className="bg-brand text-brand-contrast hover:bg-brand-hover mt-10 inline-flex h-12 items-center justify-center rounded-full px-8 font-semibold transition-colors"
      >
        トップへ戻る
      </Link>
    </div>
  );
}
