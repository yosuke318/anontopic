import Link from "next/link";
import type { Metadata } from "next";

import { LegalSection } from "@/components/legal-section";
import { SiteChrome } from "@/components/site-chrome";
import { contactEmail, operatorName, siteName } from "@/lib/site";

const description = `${siteName} の運営者情報と、通報・権利侵害の申し立て・お問い合わせの窓口です。`;

export const metadata: Metadata = {
  title: "運営者情報・お問い合わせ",
  description,
  alternates: { canonical: "/contact" },
  openGraph: {
    title: `運営者情報・お問い合わせ | ${siteName}`,
    description,
    url: "/contact",
  },
};

export default function ContactPage() {
  return (
    <SiteChrome>
      <article className="mx-auto w-full max-w-3xl px-5 py-16">
        <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">運営者情報・お問い合わせ</h1>

        <LegalSection id="operator" title="運営者">
          <dl className="grid gap-x-6 gap-y-3 sm:grid-cols-[8rem_1fr]">
            <dt className="text-foreground font-bold">サービス名</dt>
            <dd>{siteName}</dd>
            <dt className="text-foreground font-bold">運営者</dt>
            <dd>{operatorName}（個人で運営しています）</dd>
            <dt className="text-foreground font-bold">氏名・住所</dt>
            <dd>求めがあれば、下のメールアドレスあてに遅滞なく回答します。</dd>
            <dt className="text-foreground font-bold">連絡先</dt>
            <dd>
              <a href={`mailto:${contactEmail}`} className="text-foreground underline">
                {contactEmail}
              </a>
            </dd>
          </dl>
        </LegalSection>

        <LegalSection id="channels" title="窓口">
          <dl className="space-y-5">
            <div>
              <dt className="text-foreground font-bold">会話の相手に問題がある</dt>
              <dd className="mt-1">
                会話画面の「通報」ボタンから知らせてください。通報した時点で会話は終了し、運営者が会話の内容を確認します。
              </dd>
            </div>
            <div>
              <dt className="text-foreground font-bold">会話によって自分の権利が侵害された</dt>
              <dd className="mt-1">
                <Link href="/claims" className="text-foreground underline">
                  権利侵害の申し立てフォーム
                </Link>
                から送ってください。本サービスを利用していない方も送れます。
              </dd>
            </div>
            <div>
              <dt className="text-foreground font-bold">
                そのほか（利用規約、個人情報の扱い、発信者情報の開示の請求など）
              </dt>
              <dd className="mt-1">
                <a href={`mailto:${contactEmail}`} className="text-foreground underline">
                  {contactEmail}
                </a>{" "}
                にメールで連絡してください。個人で運営しているため、回答までに数日かかることがあります。
              </dd>
            </div>
          </dl>
        </LegalSection>
      </article>
    </SiteChrome>
  );
}
