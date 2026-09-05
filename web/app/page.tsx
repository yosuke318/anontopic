import Link from "next/link";
import type { Metadata } from "next";

import { absoluteUrl, siteDescription, siteName, siteTagline } from "@/lib/site";

export const metadata: Metadata = {
  alternates: { canonical: "/" },
};

const lead = `${siteName} は、選んだトピックについて匿名でテキストチャットできるサービスです。会員登録はいりません。雑談も、趣味の話も、少し聞いてほしいことも、話題を選ぶところから始まります。`;

const features = [
  {
    title: "会員登録がいらない",
    body: "メールアドレスもプロフィールも登録しません。ブラウザを開いてトピックを選ぶだけで話しはじめられます。",
  },
  {
    title: "話題から相手が決まる",
    body: "相手を探すのではなく、話したいトピックを選びます。同じトピックを選んだ人と、その話題について話します。",
  },
  {
    title: "2 人と 3 人から選べる",
    body: "1 対 1 でじっくり話すか、3 人で会話するかを選べます。3 人ルームでは会話が途切れにくくなります。",
  },
];

const steps = [
  {
    title: "トピックを選ぶ",
    body: "雑談、趣味、相談など、いま話したい話題をひとつ選びます。",
  },
  {
    title: "相手を待つ",
    body: "同じトピックを選んだ人が集まるまで待ちます。待っている間はいつでもやめられます。",
  },
  {
    title: "匿名で話す",
    body: "名前も属性も伝えずに、選んだ話題について話します。会話は退出すると終わります。",
  },
];

const rooms = [
  {
    title: "2 人ルーム",
    body: "相手はひとりだけです。相談や、ひとつの話題を掘り下げたいときに向いています。相手が退出すると会話は終わります。",
  },
  {
    title: "3 人ルーム",
    body: "自分を含めて 3 人で話します。会話が途切れにくく、話しづらい相手がいたときにも第三者がいます。人数が揃わないときは、しばらく待ってから 2 人で始まります。",
  },
];

const safeguards = [
  {
    title: "送信内容のチェック",
    body: "連絡先の交換や性的な内容など、禁止している表現は送信前に止まります。",
  },
  {
    title: "通報できる",
    body: "困る相手に会ったら通報できます。通報が重なった利用者は段階的に利用を制限します。",
  },
  {
    title: "会話は残さない",
    body: "会話ログは 90 日で自動的に削除します。過去の会話を読み返す機能は持ちません。",
  },
];

const purpose = `出会い・交際・性的な目的での利用、対面の約束、連絡先の交換は禁止しています。性別や年齢、地域で相手を指定する機能も持ちません。${siteName} は、話題について話すためだけの場所です。`;

const closing = "トピックを選ぶだけで始められます。登録も、プロフィールもいりません。";

export default function HomePage() {
  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "WebSite",
    name: siteName,
    url: absoluteUrl("/"),
    description: siteDescription,
    inLanguage: "ja",
  };

  return (
    <>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd).replace(/</g, "\\u003c") }}
      />

      <section className="mx-auto w-full max-w-5xl px-5 py-16 sm:py-24">
        <p className="text-brand text-sm font-semibold tracking-wide">{siteTagline}</p>
        <h1 className="mt-4 max-w-2xl text-4xl leading-tight font-bold tracking-tight sm:text-5xl">
          話したいことだけを持って、
          <br />
          知らない誰かと話す。
        </h1>
        <p className="text-muted mt-6 max-w-xl text-lg leading-8">{lead}</p>
        <div className="mt-10 flex flex-col gap-3 sm:flex-row">
          <Link
            href="/topics"
            className="bg-brand text-brand-contrast hover:bg-brand-hover inline-flex h-12 items-center justify-center rounded-full px-8 text-base font-semibold transition-colors"
          >
            トピックを選んで話しはじめる
          </Link>
          <Link
            href="/about"
            className="border-line hover:bg-surface inline-flex h-12 items-center justify-center rounded-full border px-8 text-base font-medium transition-colors"
          >
            どんなサービスか読む
          </Link>
        </div>
      </section>

      <section className="border-line bg-surface border-y">
        <div className="mx-auto grid w-full max-w-5xl gap-8 px-5 py-16 sm:grid-cols-3">
          {features.map((feature) => (
            <div key={feature.title}>
              <h2 className="text-lg font-bold">{feature.title}</h2>
              <p className="text-muted mt-3 leading-7">{feature.body}</p>
            </div>
          ))}
        </div>
      </section>

      <section className="mx-auto w-full max-w-5xl px-5 py-16">
        <h2 className="text-2xl font-bold tracking-tight sm:text-3xl">使い方</h2>
        <ol className="mt-8 grid gap-6 sm:grid-cols-3">
          {steps.map((step, index) => (
            <li key={step.title} className="border-line rounded-2xl border p-6">
              <span className="bg-brand-soft text-brand flex h-8 w-8 items-center justify-center rounded-full text-sm font-bold">
                {index + 1}
              </span>
              <h3 className="mt-4 font-bold">{step.title}</h3>
              <p className="text-muted mt-2 leading-7">{step.body}</p>
            </li>
          ))}
        </ol>
      </section>

      <section className="border-line bg-surface border-y">
        <div className="mx-auto w-full max-w-5xl px-5 py-16">
          <h2 className="text-2xl font-bold tracking-tight sm:text-3xl">ルームは 2 人と 3 人</h2>
          <div className="mt-8 grid gap-6 sm:grid-cols-2">
            {rooms.map((room) => (
              <div key={room.title} className="border-line bg-background rounded-2xl border p-6">
                <h3 className="font-bold">{room.title}</h3>
                <p className="text-muted mt-3 leading-7">{room.body}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="mx-auto w-full max-w-5xl px-5 py-16">
        <h2 className="text-2xl font-bold tracking-tight sm:text-3xl">安心して話すために</h2>
        <div className="mt-8 grid gap-6 sm:grid-cols-3">
          {safeguards.map((safeguard) => (
            <div key={safeguard.title}>
              <h3 className="font-bold">{safeguard.title}</h3>
              <p className="text-muted mt-3 leading-7">{safeguard.body}</p>
            </div>
          ))}
        </div>
        <p className="border-line text-muted mt-10 rounded-2xl border border-dashed p-6 leading-7">
          {purpose}
        </p>
      </section>

      <section className="border-line bg-surface border-t">
        <div className="mx-auto w-full max-w-5xl px-5 py-16 text-center">
          <h2 className="text-2xl font-bold tracking-tight sm:text-3xl">
            いま話したいことはありますか
          </h2>
          <p className="text-muted mt-4 leading-7">{closing}</p>
          <Link
            href="/topics"
            className="bg-brand text-brand-contrast hover:bg-brand-hover mt-8 inline-flex h-12 items-center justify-center rounded-full px-8 text-base font-semibold transition-colors"
          >
            トピックを選ぶ
          </Link>
        </div>
      </section>
    </>
  );
}
