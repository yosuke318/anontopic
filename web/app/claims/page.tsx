import type { Metadata } from "next";

import { ClaimForm } from "@/components/claim-form";
import { SiteChrome } from "@/components/site-chrome";
import { siteName } from "@/lib/site";

const description = `${siteName} の会話の中で、ご自身の権利が侵害されていると考える方からの申し立てを受け付けます。サービスを利用していない方も送信できます。`;

export const metadata: Metadata = {
  title: "権利侵害の申し立て",
  description,
  alternates: { canonical: "/claims" },
  openGraph: {
    title: `権利侵害の申し立て | ${siteName}`,
    description,
    url: "/claims",
  },
};

const lead = `${siteName} の会話の中で、プライバシーや名誉などご自身の権利が侵害されていると考える方は、このフォームからお知らせください。サービスを利用していない方も送信できます。`;

const reportInstead =
  "会話に参加している方は、会話画面の「通報」ボタンをお使いください。この会話を運営が確認できるよう、対象の会話と結びつけて受け付けます。";

const includeItems = [
  "どのような情報が書き込まれていたか（例: 本名、住所、勤務先、事実でない内容）",
  "いつごろの会話か、どこでそれを知ったか",
  "その情報によってどの権利が侵害されていると考えるか",
];

const handling =
  "会話は参加者どうしでしか見えないため、該当する会話を運営側で探して確認します。いただいたメールアドレスは、この申し立てへの連絡にだけ使います。申し立ての記録は、法令にもとづく対応のため長期間保管します。";

export default function ClaimsPage() {
  return (
    <SiteChrome>
      <article className="mx-auto w-full max-w-3xl px-5 py-16">
        <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">権利侵害の申し立て</h1>
        <p className="text-muted mt-6 text-lg leading-8">{lead}</p>
        <p className="text-muted mt-4 leading-7">{reportInstead}</p>

        <section className="mt-12">
          <h2 className="text-xl font-bold tracking-tight">書いていただきたいこと</h2>
          <ul className="text-muted mt-4 list-disc space-y-2 pl-5 leading-7">
            {includeItems.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
          <p className="text-muted mt-6 leading-7">{handling}</p>
        </section>

        <ClaimForm />
      </article>
    </SiteChrome>
  );
}
