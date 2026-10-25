# ADR-0040: ドメインを Cloudflare で登録し、サブドメインを Route 53 に委任する

- ステータス: Accepted
- 日付: 2026-10-25
- 関連 issue: YOSUKE-145, YOSUKE-144

## 文脈

- Terraform は Route 53 のパブリックホストゾーンを参照し、ACM の検証レコード・API の
  レコード・CloudFront への ALIAS レコードをそこに作る（compute / web モジュール）。
  ホストゾーン自体は Terraform では作らない。
- この AWS アカウントは、新しいドメインの登録が制限されている。2026-10 に登録が理由の
  表示なしに 5 回続けて失敗し、AWS サポートから「制限は解除できず、時期も示せない。
  別のレジストラで登録して Route 53 で DNS をホストするか、AWS に転送することはできる」
  という回答を受けた。
- Cloudflare Registrar で登録したドメインは、Cloudflare のネームサーバーしか使えない。
  サブドメインだけを別のネームサーバーに委任することはできる。登録から 60 日間は
  他のレジストラに転送できない。
- 運用者は Cloudflare の操作に慣れている。

## 決定

- `anonymtopic.com` を Cloudflare Registrar で登録する。apex の DNS は Cloudflare に置く。
- `app.anonymtopic.com` を Route 53 のパブリックホストゾーンとして運用者が手で作り、
  Cloudflare の DNS に `app` の NS レコードを書いて委任する。
- dev と prod の `domain_name` は `app.anonymtopic.com` にする。サイトは prod が
  `app.anonymtopic.com`、dev が `dev.app.anonymtopic.com`、API はそれぞれ `api.` を前に付ける。
- ホストゾーンは Terraform で作らない。dev と prod は別の state で同じゾーンを使い、
  委任先のネームサーバーは Cloudflare に手で書くため、環境を destroy してもゾーンを
  残しておく必要がある。

## 検討した代替案

### 案A: DNS をすべて Cloudflare に置く

apex でサイトを出せるが、ACM の検証と各レコードを Terraform の cloudflare プロバイダで
書き直すことになる。dev と prod の両方の compute / web モジュールが変わり、API トークンを
GitHub に置く必要も出る。

### 案B: NS を外部に向けられるレジストラで登録する

apex をそのまま Route 53 に置け、コードも変わらない。運用者が使い慣れていない
レジストラのアカウントを作ることになる。

### 案C: Cloudflare で登録し、60 日後に Route 53 へ転送する

サポートの回答では転送は制限されないが、60 日の間ドメインの設定を進められない。

## 影響

- サイトの URL が 1 段深くなる。apex（`anonymtopic.com`）には何も無いため、使うなら
  Cloudflare のリダイレクトルールで `app.anonymtopic.com` に向ける。
- DNS の設定が Cloudflare（登録・apex・委任の NS）と Route 53（`app` 以下）の 2 か所に
  分かれる。Cloudflare の `app` の NS レコードを消すと、サイトも API も引けなくなる。
- ホストゾーンの料金 月 $0.50 がかかる。
- 見直す条件: apex でサイトを出したくなったときは、60 日を過ぎてから Route 53 か
  NS を外部に向けられるレジストラに転送し、ホストゾーンを `anonymtopic.com` で作り直す。
