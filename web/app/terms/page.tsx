import Link from "next/link";
import type { Metadata } from "next";

import { LegalList, LegalSection } from "@/components/legal-section";
import { SiteChrome } from "@/components/site-chrome";
import { operatorName, siteName } from "@/lib/site";
import { termsEffectiveDate } from "@/lib/terms";

const description = `${siteName} の利用規約です。サービスの目的、禁止している行為、利用の制限、会話の記録の扱いを定めています。`;

export const metadata: Metadata = {
  title: "利用規約",
  description,
  alternates: { canonical: "/terms" },
  openGraph: {
    title: `利用規約 | ${siteName}`,
    description,
    url: "/terms",
  },
};

const prohibited = [
  "出会い・交際・性的な関係・対面を目的とした利用",
  "性別・年齢・地域などの属性によって相手を指定したり、探したりすること",
  "待ち合わせ、宿泊、飲酒、交際、性交渉に誘うこと",
  "電話番号、メールアドレス、SNS の ID、住所、位置情報などを送ったり、求めたりすること",
  "前の各号を助長する表現、隠語、外部のサービスへの誘導",
  "性的な表現、暴力的な表現、差別的な表現",
  "他人の名誉・信用・プライバシー・著作権その他の権利を侵害すること",
  "本名、住所、勤務先など、自分や他人を特定できる情報を書き込むこと",
  "嫌がらせ、脅迫、つきまとい、なりすまし",
  "宣伝、勧誘、営業、同じ内容の繰り返しの送信",
  "利用の制限を、別の端末や回線を使うなどして逃れること",
  "自動化したプログラムによる利用、本サービスへの不正なアクセス、運営を妨げる行為",
  "法令または公序良俗に反する行為、犯罪につながる行為",
];

const minorSafeguards = [
  "性別・年齢・地域などの属性を登録・表示する機能を持たず、相手を選ぶこともできません。",
  "相手は同じトピックを選んだ人の中から自動で割り当てます。",
  "連絡先や性的な表現など、禁止している表現は送信される前に止めます。",
  "会話は参加者どうしにしか見えず、会話が終わった後に読み返す機能はありません。",
];

const sanctions = [
  "禁止している表現を含むメッセージは、相手に届く前に止めます。",
  "参加者から通報があった会話は、その時点で終了します。",
  "禁止している表現の送信や通報が重なった端末には、警告、一時的な利用停止、無期限の利用停止を順に行います。",
  "運営者は、会話の内容を確認したうえで、特定の端末や接続元からの利用を制限することがあります。",
];

export default function TermsPage() {
  return (
    <SiteChrome>
      <article className="mx-auto w-full max-w-3xl px-5 py-16">
        <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">利用規約</h1>
        <p className="text-muted mt-6 leading-8">
          この利用規約（以下「本規約」）は、{operatorName}
          （以下「運営者」）が提供する匿名テキストチャットサービス「
          {siteName}
          」（以下「本サービス」）の利用条件を定めるものです。本サービスを利用する方（以下「利用者」）は、本規約に同意したうえで利用してください。
        </p>

        <LegalSection id="purpose" title="第1条（本サービスの目的）">
          <p>
            本サービスは、利用者が選んだトピックについて、雑談・趣味・相談のために匿名でテキストチャットをする場です。
          </p>
          <p>
            本サービスは、異性の紹介、出会い、交際の仲介を目的としたものではありません。利用者の性別・年齢・地域などの属性を登録・表示・検索する機能を持たず、利用者どうしが会話の外で連絡を取り合うための機能も持ちません。
          </p>
        </LegalSection>

        <LegalSection id="consent" title="第2条（本規約への同意）">
          <p>
            利用者は、本サービスで会話を始める前に本規約と
            <Link href="/privacy" className="text-foreground underline">
              プライバシーポリシー
            </Link>
            に同意するものとします。会話を始めた時点で、本規約に同意したものとみなします。
          </p>
        </LegalSection>

        <LegalSection id="minors" title="第3条（未成年の方の利用）">
          <p>
            本サービスは年齢を問わず利用できます。年齢の確認は行わない代わりに、次のとおり機能を限ることで、未成年の方が安全に利用できるようにしています。
          </p>
          <LegalList items={minorSafeguards} />
          <p>未成年の方は、保護者の方の同意を得たうえで利用してください。</p>
        </LegalSection>

        <LegalSection id="prohibited" title="第4条（禁止行為）">
          <p>利用者は、本サービスの利用にあたり、次の行為をしてはなりません。</p>
          <LegalList items={prohibited} />
        </LegalSection>

        <LegalSection id="sanctions" title="第5条（送信の制限と利用の制限）">
          <p>運営者は、本規約を守っていただくために、次の措置をとります。</p>
          <LegalList items={sanctions} />
          <p>運営者は、これらの措置の理由を利用者に説明する義務を負いません。</p>
        </LegalSection>

        <LegalSection id="reports" title="第6条（通報と権利侵害の申し立て）">
          <p>
            会話の相手に問題のある行為があった場合は、会話画面の「通報」ボタンから運営者に知らせることができます。
          </p>
          <p>
            本サービスの会話によって自分の権利が侵害されていると考える方は、本サービスを利用していない方も含め、
            <Link href="/claims" className="text-foreground underline">
              権利侵害の申し立てフォーム
            </Link>
            から申し立てることができます。運営者は内容を確認し、法令にもとづいて必要な措置をとります。
          </p>
        </LegalSection>

        <LegalSection id="records" title="第7条（会話の記録）">
          <p>
            会話のメッセージは、送信から 90
            日で自動的に削除します。通報があった会話のメッセージは、対応と記録のために例外的に保管します。取得する情報と保管する期間の詳細は
            <Link href="/privacy" className="text-foreground underline">
              プライバシーポリシー
            </Link>
            に定めます。
          </p>
        </LegalSection>

        <LegalSection id="changes-to-service" title="第8条（本サービスの変更・中断・終了）">
          <p>
            運営者は、事前に知らせることなく、本サービスの内容を変更し、または本サービスの提供を中断・終了することがあります。
          </p>
        </LegalSection>

        <LegalSection id="disclaimer" title="第9条（免責）">
          <p>
            本サービスで交わされる会話の内容と、利用者どうしの間で生じたトラブルについて、運営者は責任を負いません。利用者どうしで解決してください。
          </p>
          <p>
            運営者は、本サービスが中断なく利用できること、不具合が無いことを保証しません。本サービスの利用によって利用者に生じた損害について、運営者は責任を負いません。ただし、運営者の故意または重大な過失による場合は、この限りではありません。
          </p>
        </LegalSection>

        <LegalSection id="changes-to-terms" title="第10条（本規約の変更）">
          <p>
            運営者は、必要に応じて本規約を変更することがあります。変更する場合は、変更後の内容と効力が生じる日を本サービス上に掲示します。効力が生じた日以降に本サービスを利用した場合は、変更後の本規約に同意したものとみなします。
          </p>
        </LegalSection>

        <LegalSection id="jurisdiction" title="第11条（準拠法と管轄裁判所）">
          <p>
            本規約は日本法に準拠します。本サービスに関して紛争が生じた場合は、運営者の住所地を管轄する地方裁判所を第一審の専属的合意管轄裁判所とします。
          </p>
        </LegalSection>

        <p className="text-muted mt-16 text-sm">制定日: {termsEffectiveDate}</p>
      </article>
    </SiteChrome>
  );
}
