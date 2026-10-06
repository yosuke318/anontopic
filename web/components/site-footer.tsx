import Link from "next/link";

import { siteName } from "@/lib/site";

const purpose = `${siteName} は、雑談・趣味・相談のための匿名テキストチャットです。出会いや交際を目的とした利用はできません。`;

const links = [
  { href: "/about", label: "サービスについて" },
  { href: "/topics", label: "トピックを選ぶ" },
  { href: "/terms", label: "利用規約" },
  { href: "/privacy", label: "プライバシーポリシー" },
  { href: "/claims", label: "権利侵害の申し立て" },
  { href: "/contact", label: "運営者情報・お問い合わせ" },
];

export function SiteFooter() {
  return (
    <footer className="border-line mt-auto border-t">
      <div className="text-muted mx-auto flex w-full max-w-5xl flex-col gap-4 px-5 py-8 text-sm">
        <p>{purpose}</p>
        <nav aria-label="フッター" className="flex flex-wrap gap-x-5 gap-y-2">
          {links.map((link) => (
            <Link
              key={link.href}
              href={link.href}
              className="hover:text-foreground whitespace-nowrap transition-colors"
            >
              {link.label}
            </Link>
          ))}
        </nav>
      </div>
    </footer>
  );
}
