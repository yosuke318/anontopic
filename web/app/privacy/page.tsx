import Link from "next/link";
import type { Metadata } from "next";

import { LegalList, LegalSection } from "@/components/legal-section";
import { SiteChrome } from "@/components/site-chrome";
import { contactEmail, operatorName, siteName } from "@/lib/site";
import { privacyRevisedDate, termsEffectiveDate } from "@/lib/terms";

const description = `${siteName}のプライバシーポリシーです。取得する情報、利用の目的、保管する期間、広告の配信のために外部に送信される情報、開示の請求への対応を説明します。`;

export const metadata: Metadata = {
  title: "プライバシーポリシー",
  description,
  alternates: { canonical: "/privacy" },
  openGraph: {
    title: `プライバシーポリシー | ${siteName}`,
    description,
    url: "/privacy",
  },
};

const collected = [
  {
    name: "セッションの Cookie（anontopic_session）",
    purpose:
      "会員登録なしで会話に入るための匿名のセッションを識別します。最後の利用から 24 時間、または発行から 7 日で無効になります。",
  },
  {
    name: "端末 ID の Cookie（anontopic_device）",
    purpose:
      "ブラウザごとに発行する乱数です。禁止している行為を繰り返す端末に利用の制限をかけるために使います。発行から 1 年で無効になります。",
  },
  {
    name: "接続元の IP アドレス",
    purpose:
      "荒らしや不正な接続の検知、送信回数の制限、利用の制限に使います。IP アドレスはそのまま保存せず、鍵付きのハッシュ値に変えてから保存します。鍵は運営者だけが管理し、鍵を持たない人がハッシュ値から IP アドレスを知ることはできません。",
  },
  {
    name: "送信したメッセージ",
    purpose:
      "会話の相手に届けるほか、禁止している表現の判定と、通報があったときの確認に使います。禁止している表現を含むために相手に届かなかったメッセージも記録します。",
  },
  {
    name: "通報の内容",
    purpose: "通報の理由と対象の会話を、運営者が確認と対応に使います。",
  },
  {
    name: "権利侵害の申し立ての内容",
    purpose:
      "氏名または名称、メールアドレス、申し立ての内容を、確認と、申し立てた方への連絡に使います。",
  },
  {
    name: "お問い合わせのメール",
    purpose: "メールアドレスと内容を、お問い合わせへの回答に使います。",
  },
];

const retention = [
  "メッセージは、送信から 90 日で自動的に削除します。",
  "通報があった会話のメッセージは、対応と記録のため、90 日を過ぎても削除せずに保管します。",
  "会話に参加した記録のうち、ハッシュ値にした接続元と端末 ID は、不正な利用の防止と法令にもとづく対応のため、参加から 6 か月保管し、その後に削除します。通報があった会話の参加者の分は、6 か月を過ぎても保管します。",
  "利用の制限の記録（制限をかけた接続元のハッシュ値と端末 ID）は、制限を続けるため、会話の記録とは別に保管します。",
  "通報と権利侵害の申し立ての記録は、法令にもとづく対応のため保管します。",
];

const processors = [
  "Amazon Web Services（本サービスを動かすサーバーとデータベース。東京リージョン）",
  "Grafana Cloud（サーバーの稼働状況の監視。メッセージの本文、セッションの識別子、IP アドレスは送りません）",
  "Google AdSense（広告の配信。「4. 広告の配信と外部への情報の送信」を参照）",
];

const adTransmission = [
  {
    name: "送信される情報",
    detail:
      "Cookie などの識別子、IP アドレス、閲覧しているページの URL、ブラウザと端末の種類、広告の表示とクリックの記録",
  },
  { name: "送信先", detail: "Google LLC" },
  {
    name: "利用の目的",
    detail:
      "広告の配信、閲覧の履歴にもとづく広告（パーソナライズ広告）の表示、広告の効果の測定、不正なクリックの検知",
  },
];

const externalLinkClassName = "text-foreground underline";

export default function PrivacyPage() {
  return (
    <SiteChrome>
      <article className="mx-auto w-full max-w-3xl px-5 py-16">
        <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">プライバシーポリシー</h1>
        <p className="text-muted mt-6 leading-8">
          {operatorName}（以下「運営者」）は、匿名テキストチャットサービス「{siteName}
          」（以下「本サービス」）で取得する情報を、このプライバシーポリシーに従って扱います。本サービスは会員登録を求めず、名前やプロフィールも登録しません。
        </p>

        <LegalSection id="collected" title="1. 取得する情報と利用の目的">
          <dl className="space-y-5">
            {collected.map((item) => (
              <div key={item.name}>
                <dt className="text-foreground font-bold">{item.name}</dt>
                <dd className="mt-1">{item.purpose}</dd>
              </div>
            ))}
          </dl>
          <p>
            利用規約に同意したことは、お使いのブラウザの中にだけ記録し、運営者には送りません。会話の相手をブロックした記録も、ブラウザの中にだけ持ちます。
          </p>
        </LegalSection>

        <LegalSection id="retention" title="2. 保管する期間">
          <LegalList items={retention} />
        </LegalSection>

        <LegalSection id="third-parties" title="3. 第三者への提供と外部のサービス">
          <p>
            運営者は、法令にもとづく場合を除き、取得した情報を本人の同意なく第三者に提供しません。本サービスは次の外部のサービスを利用しています。
          </p>
          <LegalList items={processors} />
        </LegalSection>

        <LegalSection id="ads" title="4. 広告の配信と外部への情報の送信">
          <p>
            本サービスは、トップページと「サービスについて」のページに、Google LLC
            が提供する広告配信サービス Google AdSense
            の広告を表示します。会話の画面と待機画面には広告を表示しません。
          </p>
          <p>
            広告を表示するページを開くと、お使いのブラウザから Google
            に次の情報が送信されます。運営者はこれらの情報を受け取りません。また、会話のメッセージを
            Google に提供することはありません。
          </p>
          <dl className="space-y-5">
            {adTransmission.map((item) => (
              <div key={item.name}>
                <dt className="text-foreground font-bold">{item.name}</dt>
                <dd className="mt-1">{item.detail}</dd>
              </div>
            ))}
          </dl>
          <p>
            Google を含む第三者配信事業者は Cookie
            を使い、利用者が本サービスや他のウェブサイトに過去にアクセスした際の情報にもとづいて広告を配信します。Google
            は広告の Cookie を使うことで、Google
            とそのパートナーが、本サービスや他のウェブサイトへのアクセスの情報にもとづいた広告を表示できるようにしています。Google
            による情報の扱いは、
            <a
              href="https://policies.google.com/technologies/partner-sites?hl=ja"
              target="_blank"
              rel="noopener noreferrer"
              className={externalLinkClassName}
            >
              Google のサービスを使用するサイトやアプリから収集した情報の Google による使用
            </a>
            を参照してください。
          </p>
          <p>
            パーソナライズ広告は、Google の
            <a
              href="https://adssettings.google.com/"
              target="_blank"
              rel="noopener noreferrer"
              className={externalLinkClassName}
            >
              広告設定
            </a>
            で無効にできます。第三者配信事業者の Cookie は、
            <a
              href="https://optout.aboutads.info/"
              target="_blank"
              rel="noopener noreferrer"
              className={externalLinkClassName}
            >
              www.aboutads.info
            </a>
            でも無効にできます。ブラウザの設定で Cookie を拒否することもできます。
          </p>
          <p>
            欧州経済領域（EEA）、英国、スイスからアクセスした場合は、Google
            の同意を求める画面が表示され、広告の Cookie を使うかどうかを選べます。
          </p>
        </LegalSection>

        <LegalSection id="disclosure" title="5. 発信者情報の開示の請求と裁判所の命令">
          <p>
            運営者は、情報流通プラットフォーム対処法その他の法令にもとづく発信者情報の開示の請求や、裁判所の命令・捜査機関からの照会に、法令に従って対応します。
          </p>
          <p>
            運営者が保有するのは「1. 取得する情報と利用の目的」に挙げた情報のうち、「2.
            保管する期間」の間に残っているものだけです。IP
            アドレスはハッシュ値でしか保存していないため、IP アドレスそのものは開示できません。
          </p>
        </LegalSection>

        <LegalSection id="requests" title="6. 保有する情報の開示・訂正・利用停止の請求">
          <p>
            保有する個人データの開示・訂正・利用停止などを求める場合は、下の窓口に連絡してください。本サービスは匿名で利用するもので、運営者は利用者の氏名などを持っていません。そのため、どの記録がご本人のものかを確かめられる範囲で対応します。
          </p>
        </LegalSection>

        <LegalSection id="security" title="7. 安全管理のための措置">
          <LegalList
            items={[
              "ブラウザとの通信と、データベースへの接続は TLS で暗号化します。",
              "IP アドレスは鍵付きのハッシュ値にしてから保存します。",
              "データベースはインターネットから直接届かない場所に置き、扱えるのは運営者に限ります。",
            ]}
          />
        </LegalSection>

        <LegalSection id="changes" title="8. このポリシーの変更">
          <p>
            運営者は、必要に応じてこのポリシーを変更することがあります。変更した場合は、本サービス上に掲示します。
          </p>
        </LegalSection>

        <LegalSection id="contact" title="9. 運営者と窓口">
          <p>
            本サービスは {operatorName}
            が個人で運営しています。運営者の氏名と住所は、求めがあれば遅滞なく回答します。
          </p>
          <p>
            お問い合わせ:{" "}
            <a href={`mailto:${contactEmail}`} className="text-foreground underline">
              {contactEmail}
            </a>
            （
            <Link href="/contact" className="text-foreground underline">
              運営者情報・お問い合わせ
            </Link>
            ）
          </p>
        </LegalSection>

        <p className="text-muted mt-16 text-sm">
          制定日: {termsEffectiveDate}
          <br />
          改定日: {privacyRevisedDate}
        </p>
      </article>
    </SiteChrome>
  );
}
