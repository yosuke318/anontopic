import Link from "next/link";
import type { Metadata } from "next";

import { siteName } from "@/lib/site";

const description =
  "anontopic は、選んだトピックについて匿名でテキストチャットできるサービスです。使い方、ルームの種類、禁止していること、会話ログの扱いを説明します。";

export const metadata: Metadata = {
  title: "サービスについて",
  description,
  alternates: { canonical: "/about" },
  openGraph: {
    title: `サービスについて | ${siteName}`,
    description,
    url: "/about",
  },
};

const lead = `${siteName} は、話したい話題を選んで、知らない誰かと匿名でテキストチャットできるサービスです。雑談・趣味・相談のための場所として運営しています。`;

const capabilities = [
  {
    title: "トピックを選んで話す。",
    body: "用意された話題からひとつ選び、同じ話題を選んだ人と会話します。",
  },
  {
    title: "人数を選ぶ。",
    body: "1 対 1 の 2 人ルームか、自分を含めて 3 人で話す 3 人ルームかを選べます。",
  },
  {
    title: "名乗らずに話す。",
    body: "名前もプロフィールも持たず、会話が終われば相手との結びつきも残りません。",
  },
];

const rooms = [
  {
    title: "2 人ルーム",
    body: "相手はひとりです。相談ごとや、ひとつの話題をじっくり話したいときに向いています。",
  },
  {
    title: "3 人ルーム",
    body: "自分を含めて 3 人で話します。第三者がいることで会話が続きやすく、問題のあるやりとりも起きにくくなります。",
  },
];

const prohibitedLead = `${siteName} は異性紹介や出会いの仲介を目的としたサービスではありません。次の利用は禁止しています。`;

const prohibited = [
  "出会い・交際・性的な関係・対面を目的とした利用",
  "性別・年齢・地域などの属性による相手の指定や検索の要求",
  "待ち合わせ、宿泊、飲酒、交際、性交渉の勧誘",
  "電話番号、メールアドレス、SNS の ID、住所、位置情報などの送受信",
  "上記を助長する表現、隠語、外部サービスへの誘導",
];

const sanctions =
  "禁止している表現は送信前に止まります。繰り返す利用者には、警告・一時停止・恒久停止と段階的に制限をかけます。";

const retention =
  "会話ログは 90 日で自動的に削除します。通報された会話は、対応と記録のために例外的に長く保持します。荒らしや不正な接続を検知するために、接続元のアドレスはそのままではなくハッシュ化した値で扱います。";

const faq = [
  {
    question: "会員登録は必要ですか。",
    answer:
      "必要ありません。トピックを選ぶと匿名のセッションが発行され、そのまま会話に入れます。メールアドレスやプロフィールは登録しません。",
  },
  {
    question: "相手を選べますか。",
    answer:
      "選べません。性別・年齢・地域などの属性で相手を指定する機能は持っていません。同じトピックを選んだ人の中から自動で割り当てます。",
  },
  {
    question: "会話の内容は残りますか。",
    answer:
      "会話ログは 90 日で自動的に削除します。通報された会話だけは、対応のために例外的に長く保持します。過去の会話を読み返す機能はありません。",
  },
  {
    question: "相手が見つからないときはどうなりますか。",
    answer:
      "待機したまま同じトピックの人を待ちます。3 人ルームを選んでいて人数が揃わない場合は、しばらく待ってから 2 人で会話が始まります。待機はいつでもやめられます。",
  },
  {
    question: "困る相手に会ったらどうすればよいですか。",
    answer:
      "会話画面から通報できます。通報が重なった利用者には、警告・一時停止・恒久停止と段階的に制限をかけます。",
  },
];

export default function AboutPage() {
  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "FAQPage",
    mainEntity: faq.map((item) => ({
      "@type": "Question",
      name: item.question,
      acceptedAnswer: { "@type": "Answer", text: item.answer },
    })),
  };

  return (
    <>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd).replace(/</g, "\\u003c") }}
      />

      <article className="mx-auto w-full max-w-3xl px-5 py-16">
        <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">サービスについて</h1>
        <p className="text-muted mt-6 text-lg leading-8">{lead}</p>

        <section className="mt-14">
          <h2 className="text-2xl font-bold tracking-tight">できること</h2>
          <ul className="text-muted mt-6 space-y-4 leading-7">
            {capabilities.map((capability) => (
              <li key={capability.title}>
                <strong className="text-foreground">{capability.title}</strong> {capability.body}
              </li>
            ))}
          </ul>
        </section>

        <section className="mt-14">
          <h2 className="text-2xl font-bold tracking-tight">ルームの種類</h2>
          <div className="mt-6 grid gap-6 sm:grid-cols-2">
            {rooms.map((room) => (
              <div key={room.title} className="border-line rounded-2xl border p-6">
                <h3 className="font-bold">{room.title}</h3>
                <p className="text-muted mt-3 leading-7">{room.body}</p>
              </div>
            ))}
          </div>
        </section>

        <section className="mt-14">
          <h2 className="text-2xl font-bold tracking-tight">禁止していること</h2>
          <p className="text-muted mt-6 leading-7">{prohibitedLead}</p>
          <ul className="text-muted mt-6 list-disc space-y-2 pl-5 leading-7">
            {prohibited.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
          <p className="text-muted mt-6 leading-7">{sanctions}</p>
        </section>

        <section className="mt-14">
          <h2 className="text-2xl font-bold tracking-tight">会話ログの扱い</h2>
          <p className="text-muted mt-6 leading-7">{retention}</p>
        </section>

        <section className="mt-14">
          <h2 className="text-2xl font-bold tracking-tight">よくある質問</h2>
          <dl className="mt-6 space-y-6">
            {faq.map((item) => (
              <div key={item.question} className="border-line border-b pb-6 last:border-b-0">
                <dt className="font-bold">{item.question}</dt>
                <dd className="text-muted mt-2 leading-7">{item.answer}</dd>
              </div>
            ))}
          </dl>
        </section>

        <div className="mt-14">
          <Link
            href="/topics"
            className="bg-brand text-brand-contrast hover:bg-brand-hover inline-flex h-12 items-center justify-center rounded-full px-8 text-base font-semibold transition-colors"
          >
            トピックを選ぶ
          </Link>
        </div>
      </article>
    </>
  );
}
