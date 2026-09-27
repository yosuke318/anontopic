# ADR-0027: Terraform を公開リポジトリに置き、dev と prod を 1 つの AWS アカウントの中で分ける

- ステータス: Accepted
- 日付: 2026-09-27
- 関連 issue: YOSUKE-134

## 文脈

- このリポジトリは GitHub で公開しており、Actions のログも誰でも読める。
- 開発体制は 1 名、月額予算は 4 万円前後（設計書 2 章）。GitHub は Free プランを使う。
- 匿名チャットは荒らしや制裁逃れの標的になりやすく、ネットワーク構成や閾値のような
  下調べに使える情報は公開しない方がよい。
- 同じ AWS アカウントを他のプロジェクトも使っており、GitHub の OIDC プロバイダは既にある。
  OIDC プロバイダは URL ごとにアカウントに 1 つしか作れない。
- 以降のインフラ issue はこの構成の上に乗る。

## 決定

- Terraform は `infra/` に置き、公開リポジトリのまま管理する。アカウント ID・state バケット名
  などアカウント固有の値はコードに書かず、Git 管理外の `terraform.tfvars` / `backend.tfbackend`
  と GitHub のシークレットから渡す。
- dev と prod は 1 つの AWS アカウントに置き、state のキー（`dev/` / `prod/`）・リソース名・
  `Env` タグで分ける。プロバイダの `allowed_account_ids` で別アカウントへの適用を止める。
- state は S3 に置き、ロックは S3 のネイティブロック（`use_lockfile`）を使う。
  バケットは `infra/bootstrap` で作り、bootstrap 自身の state はローカルに置く。
- GitHub Actions は OIDC でロールを引き受ける。plan 用（`ReadOnlyAccess` とロックファイルの
  書き込み）は pull request と `main` から、apply 用（`AdministratorAccess`）は Environment
  `dev` / `prod` からだけ引き受けられる。長期のアクセスキーは作らない。
  OIDC プロバイダは作らず、既存のものを data source で参照する。
- CI の plan はログに件数の行だけを出す。差分の中身はローカルで確かめる。

## 検討した代替案

### 案A: Terraform を private の別リポジトリに分ける

構成そのものは隠せる。ただし GitHub Free では private リポジトリにブランチ保護をかけられず、
Actions の無料枠も月 2,000 分に限られる。アプリとインフラにまたがる変更が PR 2 本に割れる。
隠したいのは構成より閾値や ID のような値で、それはリポジトリの外に出せば足りる。

### 案B: dev と prod を別の AWS アカウントに分ける

影響範囲と請求が完全に分かれる。ただし bootstrap（state バケット・OIDC プロバイダ・ロール）を
アカウントごとに回す必要があり、1 名の運用では初期構築と保守の手間に見合わない。

### 案C: ロックに DynamoDB を使う

Terraform 1.10 以降は S3 だけでロックできる。テーブルを 1 つ余計に作って管理する理由がない。

### 案D: bootstrap の state も作ったバケットに移す

移すとバケットを壊したときに bootstrap も動かせなくなる。bootstrap のリソースは少なく、
state を失っても `import` で取り込み直せる。

### 案E: bootstrap で OIDC プロバイダを作る（既存のものを import する）

他のプロジェクトと共有しているため、この構成で destroy すると他のプロジェクトの CI が止まる。

### 案F: CI の plan の出力をそのままログに出す

公開ログにリソースの属性（ARN・設定値）がそのまま載り、値をリポジトリに書かない意味がなくなる。

## 影響

- apply ロールは dev と prod で共通で、dev のジョブからも prod のリソースに触れる。dev と prod の
  分離は state・命名・タグの規律と、GitHub の Environment の保護ルールに頼る。
- plan ロールの `ReadOnlyAccess` は全 S3 オブジェクトを読める。引き受けられるのは同じリポジトリの
  ブランチからの pull request だけで、ブランチを作れるのは開発者本人に限られる。
- CI の plan が通るには、bootstrap の適用とシークレットの登録が先に要る。
- bootstrap の state は開発者のマシンにしかない。
- 見直す条件: 開発者が増えたとき、または dev の操作が prod に影響する事故が起きたときは、
  案B（アカウント分割）を再検討する。
