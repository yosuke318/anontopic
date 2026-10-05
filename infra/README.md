# infra

AWS のインフラを Terraform で管理する。dev と prod は 1 つの AWS アカウントの中で、
state のキー・リソース名の接頭辞・タグで分ける（ADR-0027）。

```
infra/
├── bootstrap/          # state バケットと GitHub Actions 用の IAM ロール。state はローカルに置く
├── environments/
│   ├── dev/            # state: s3://<バケット>/dev/terraform.tfstate
│   └── prod/           # state: s3://<バケット>/prod/terraform.tfstate
└── modules/
    ├── network/
    ├── compute/
    ├── web/
    ├── database/
    ├── cache/
    └── monitoring/
```

## リポジトリに置かない値

このリポジトリは公開している。アカウント ID・state バケット名のようなアカウント固有の値は
コードに書かず、Git 管理外のファイルか GitHub のシークレットから渡す。

| ファイル | 中身 | ひな形 |
| --- | --- | --- |
| `terraform.tfvars` | 変数の値（`aws_account_id`・`domain_name` など） | `example.tfvars` |
| `backend.tfbackend` | state バケット名 | `example.tfbackend` |

どちらも `.gitignore` で除外している。`plan` の出力にも同じ値が出るため、貼り付ける先に気をつける。

## タグ

プロバイダの `default_tags` で、すべてのリソースに次のタグを付ける。

| キー | 値 |
| --- | --- |
| `Project` | `anontopic` |
| `Env` | `dev` / `prod`（bootstrap は `shared`） |
| `ManagedBy` | `terraform` |

## 初回の構築

### 1. bootstrap を適用する

管理者権限の認証情報で実行する。bootstrap の state はこのディレクトリの
`terraform.tfstate` に残る。消した場合は `import` ブロックで取り込み直す。

GitHub の OIDC プロバイダは作らず、アカウントにある既存のものを参照する。無い場合は
先に作っておく。

```bash
cd infra/bootstrap
cp example.tfvars terraform.tfvars   # アカウント ID とバケット名を入れる
terraform init
terraform apply
```

### 2. GitHub にシークレットを登録する

リポジトリの Settings → Secrets and variables → Actions に登録する。

| 名前 | 値 |
| --- | --- |
| `AWS_ACCOUNT_ID` | AWS アカウント ID |
| `TF_STATE_BUCKET` | bootstrap の出力 `state_bucket_name` |
| `ALARM_EMAIL` | アラームと予算の通知を受け取るメールアドレス（prod の `alarm_email` と同じ値） |

ドメインを取得したら、同じ画面の Variables に `DOMAIN_NAME`（Route 53 のホストゾーン名）を
登録する。未登録の間、CI の plan は証明書・DNS レコード・HTTPS リスナー・フロントエンドを
作らない構成で取り、`Deploy` ワークフローはフロントエンドのビルドを飛ばす。
ローカルの `terraform.tfvars` の `domain_name` と揃えないと、CI とローカルで plan が食い違う。

CI の `インフラ plan` ジョブは `anontopic-terraform-plan` ロールを OIDC で引き受けて、
dev と prod の plan を取る。ログには件数の行だけを出す。

`anontopic-terraform-apply` ロールと `anontopic-deploy` ロールは、GitHub の Environment
`dev` / `prod` を指定したジョブからだけ引き受けられる。Settings → Environments で 2 つを作り、
保護ルールを付ける。

| Environment | Deployment branches | Required reviewers |
| --- | --- | --- |
| `dev` | `main` だけ | なし |
| `prod` | `main` だけ | 運用者 |

### 3. 各環境を初期化する

```bash
cd infra/environments/dev
cp example.tfvars terraform.tfvars
cp example.tfbackend backend.tfbackend
cd ../../..
make infra-plan INFRA_ENV=dev
```

prod も同じ手順で `INFRA_ENV=prod` にする。

## 日々の作業

```bash
make infra-fmt                # 整形
make infra-check              # fmt の検査と validate（AWS の認証情報は要らない）
make infra-plan INFRA_ENV=dev  # リモートの state に対して plan を取る
```

dev は追加開発のときだけ作り、使い終わったら destroy する。dev の ECR リポジトリは
イメージが残っていても削除できる。作り直したときは、下の「アプリの実行環境」の初回の構築と
「アプリのロールを作る」をやり直す。

```bash
terraform -chdir=infra/environments/dev destroy
```

## ネットワーク

プライベートサブネットからの外向き通信は、パブリックサブネットの NAT インスタンス 1 台を通る
（ADR-0028）。SSH は開けていないため、入るときは Session Manager を使う。

AMI は作成時のものに固定している。OS を更新するときは入れ替える。入れ替えの間は
プライベートサブネットから外に出られない。

```bash
terraform -chdir=infra/environments/dev apply -replace=module.network.aws_instance.nat
```

## アプリの実行環境

Go サーバーは ECS Fargate（ARM64）で動かし、ALB で HTTPS / WSS を終端する（ADR-0029）。
タスク数は固定で、prod は 2、dev は 1。API のドメインは prod が `api.<domain_name>`、
dev が `api.dev.<domain_name>`。`domain_name` が未設定の間は ALB にリスナーが無く、外から届かない。

Next.js のフロントエンドは同じクラスターで 1 タスク動かし、CloudFront から配る（ADR-0035）。
サイトのドメインは prod が `<domain_name>`、dev が `dev.<domain_name>`。`domain_name` が
未設定の間は、フロントエンドの環境も CloudFront も作らない。

ホストゾーンはこの構成では作らない。ドメインを取得したときに作られたものを参照する。

### 秘密の環境変数

サーバーの秘密は SSM パラメータストアの SecureString に置き、Terraform では扱わない（ADR-0030）。
タスクは起動時に読むため、無いと起動に失敗する。環境を作る前に入れておく。

| パラメータ | 中身 |
| --- | --- |
| `/anontopic/<env>/api/DATABASE_URL` | PostgreSQL の接続 URL |
| `/anontopic/<env>/api/REDIS_URL` | Redis の接続 URL |
| `/anontopic/<env>/api/SESSION_IP_HASH_SECRET` | IP アドレスのハッシュに使う鍵 |
| `/anontopic/<env>/api/ADMIN_API_TOKEN` | 管理 API の Bearer トークン |

```bash
aws ssm put-parameter --type SecureString \
  --name /anontopic/dev/api/SESSION_IP_HASH_SECRET --value "$(openssl rand -hex 32)"
```

値を変えたら、新しいデプロイでタスクを入れ替える。

### 初回の構築

サービスを作る時点で、タスク定義は ECR の `initial` タグのイメージを指す。先に ECR だけ作って
イメージを入れてから、残りを適用する。

```bash
terraform -chdir=infra/environments/dev apply -target=module.compute.aws_ecr_repository.api

repo="$(terraform -chdir=infra/environments/dev output -raw ecr_repository_url)"
aws ecr get-login-password | docker login --username AWS --password-stdin "${repo%%/*}"
docker buildx build --platform linux/arm64 --provenance=false --push \
  -f docker/api.release.Dockerfile -t "$repo:initial" .
```

`domain_name` を設定している環境では、フロントエンドの ECR にもイメージを入れる。
`NEXT_PUBLIC_` で始まる値はビルドのときに埋め込まれるため、環境のドメインを渡す。
dev のサイトは `https://dev.<domain_name>`、API は `https://api.dev.<domain_name>`、
prod はそれぞれ `https://<domain_name>` と `https://api.<domain_name>`。

```bash
terraform -chdir=infra/environments/dev apply -target='module.web[0].aws_ecr_repository.web'

repo="$(terraform -chdir=infra/environments/dev output -raw web_ecr_repository_url)"
docker buildx build --platform linux/arm64 --provenance=false --push \
  --build-arg NEXT_PUBLIC_SITE_URL=https://dev.<domain_name> \
  --build-arg NEXT_PUBLIC_API_BASE_URL=https://api.dev.<domain_name> \
  -f docker/web.release.Dockerfile -t "$repo:initial" .
```

最後に残りを適用する。

```bash
terraform -chdir=infra/environments/dev apply
```

すでに動いている環境に後から `domain_name` を設定したときも、フロントエンドの ECR と
`initial` のイメージを先に用意してから全体を適用する。

2 回目以降は下の「デプロイ」の流れで入れ替える。Terraform はサービスが使うリビジョンを
追わないため、Terraform でタスク定義を変えたときも、次のデプロイまで動いているタスクには
反映されない。

### デプロイ

main の CI が通ると、GitHub Actions の `Deploy` ワークフローが動く（ADR-0034）。

1. API のイメージを 1 回だけビルドする。フロントエンドのイメージは dev 用と prod 用を
   それぞれビルドする（ADR-0035）。タグはどれもコミットの SHA。
2. dev が作られていれば、API のイメージを ECR に入れてマイグレーションを流し、サービスを
   切り替える。続けてフロントエンドのサービスを切り替える。作られていなければ飛ばす。
3. prod は Environment の承認を待ってから、2 と同じことをする。

フロントエンドのサービスが無い環境（`domain_name` が未設定）では、フロントエンドの手順を飛ばす。

マイグレーションは、新しいイメージのタスク定義でコマンドを `/migrate up` に替えた ECS タスクを
1 回動かして流す。失敗するとサービスは切り替えない。ログは CloudWatch Logs の
`/ecs/anontopic-<env>-api` にあり、Actions のログにはタスクの ID だけが出る。

マイグレーションは後方互換のある変更に限る。デプロイの途中とロールバックの後は、古いイメージが
新しいスキーマで動く。列やテーブルの削除・改名は、それを使わないコードを先にデプロイしてから、
別のマイグレーションで行う。

デプロイのたびに、ファミリーの最新のタスク定義のイメージだけを差し替えた新しいリビジョンを
登録する。Terraform で変えた環境変数や秘密は、次のデプロイで取り込まれる。

### ロールバック

Actions の `Deploy` ワークフローを手動で実行し、環境と戻す先のイメージのタグを指定する。
タグはコミットの SHA を 40 文字で渡す。API とフロントエンドの両方をそのタグに戻す。
ECR には直近の 30 個のイメージが残っている。フロントエンドの ECR にそのタグが無いときは、
フロントエンドは今のものを残す。

```bash
git rev-parse <戻す先のコミット>
```

マイグレーションは戻さない。新しいスキーマのまま、古いイメージが動く。

手元から入れ替えるときは `scripts/deploy.sh` を使う（`jq` が要る）。

```bash
arn="$(scripts/deploy.sh register prod api <タグ>)"
scripts/deploy.sh update prod api "$arn"
```

フロントエンドは `api` を `web` に替える。

### Terraform の apply

PR で手元の `make infra-plan` の差分を確かめ、main にマージしてから、Actions の
`インフラ apply` ワークフローを手動で実行する。plan を取ってその plan をそのまま apply し、
ログには件数とエラーの見出しだけを出す。prod は Environment の承認を待つ。

環境を一から作るときは ECR とイメージを先に用意する必要があるため、上の「初回の構築」を
手元で行う。

### フロントエンドの配信

CloudFront のオリジンは API と同じ ALB で、ALB の証明書に合わせて API のドメインを指す。
CloudFront はオリジンへの要求にヘッダー `X-Anontopic-Route: web` を付け、ALB のリスナールールは
このヘッダーがある要求だけをフロントエンドに送る。それ以外は API に届く。

| 経路 | CloudFront のキャッシュ |
| --- | --- |
| `/_next/static/*` | する（ファイル名に内容のハッシュが入り、Next.js も immutable で返す） |
| それ以外（HTML・`sitemap.xml`・OGP 画像など） | しない。毎回タスクが返す |

HTML をキャッシュしないため、デプロイの後に CloudFront のキャッシュを消す必要は無い。

Next.js のサーバーは `/topics` を描画するときに、公開ドメインの API（`API_BASE_URL`）を
NAT インスタンス経由で呼ぶ。ログは CloudWatch Logs の `/ecs/anontopic-<env>-web` にある。

### WebSocket の接続

| 設定 | 値 | 理由 |
| --- | --- | --- |
| ALB のアイドルタイムアウト | 120 秒 | サーバーは 30 秒ごとに ping を送る。それより長くする |
| 登録解除の遅延 | 300 秒 | デプロイ中、古いタスクの接続をこの間だけ残す |
| ヘルスチェック | `/healthz` | DB や Redis が止まっても全タスクが外れないようにする |

デプロイでは新しいタスクが正常になってから古いタスクを ALB から外し、登録解除の遅延が過ぎると
残った WebSocket が切れる。ブラウザはつなぎ直し、`CHAT_REJOIN_GRACE` の間に戻れば会話は続く。
SIGTERM は登録解除の後に届き、サーバーは書き込み待ちのメッセージを記録してから終わる。

## データベースとキャッシュ

RDS PostgreSQL と ElastiCache Redis は、どちらも Single-AZ の 1 台で動かす（ADR-0031）。
インスタンスやその AZ が止まると、立て直すまでサービスも止まる。

| | dev | prod |
| --- | --- | --- |
| RDS | `db.t4g.micro`、ストレージ上限 50 GiB、バックアップ 1 日 | `db.t4g.small`、ストレージ上限 300 GiB、バックアップ 7 日 |
| Redis | `cache.t4g.micro` | `cache.t4g.micro` |

どちらも接続は TLS に限る。`DATABASE_URL` には `sslmode=require` を付け、`REDIS_URL` は
`rediss://` で始める。

### アプリのロールを作る

マスターユーザーのパスワードは RDS が Secrets Manager に持ち、定期的にローテーションされる。
アプリはマスターユーザーを使わず、運用者が作るロール `anontopic_app` で接続する（ADR-0032）。
RDS はプライベートサブネットにあるため、NAT インスタンスへの Session Manager の
ポートフォワードで入る。手元に Session Manager プラグインが要る。

```bash
tf="terraform -chdir=infra/environments/dev"

aws ssm start-session --target "$($tf output -raw nat_instance_id)" \
  --document-name AWS-StartPortForwardingSessionToRemoteHost \
  --parameters "host=$($tf output -raw database_address),portNumber=5432,localPortNumber=15432"
```

別のターミナルで、マスターユーザーとしてロールを作り、データベースの所有者にする。

```bash
tf="terraform -chdir=infra/environments/dev"
master_password="$(aws secretsmanager get-secret-value \
  --secret-id "$($tf output -raw database_master_user_secret_arn)" \
  --query SecretString --output text | jq -r .password)"
app_password="$(openssl rand -hex 32)"

PGPASSWORD="$master_password" psql "host=localhost port=15432 dbname=anontopic user=anontopic_admin sslmode=require" <<SQL
CREATE ROLE anontopic_app LOGIN PASSWORD '$app_password';
GRANT anontopic_app TO CURRENT_USER;
ALTER DATABASE anontopic OWNER TO anontopic_app;
SQL
```

同じシェルで、アプリのロールでマイグレーションを流し、接続 URL を SSM に入れる。

```bash
DATABASE_URL="postgres://anontopic_app:$app_password@localhost:15432/anontopic?sslmode=require" \
  go run ./cmd/migrate up

aws ssm put-parameter --type SecureString --overwrite \
  --name /anontopic/dev/api/DATABASE_URL \
  --value "postgres://anontopic_app:$app_password@$($tf output -raw database_address):5432/anontopic?sslmode=require"

aws ssm put-parameter --type SecureString --overwrite \
  --name /anontopic/dev/api/REDIS_URL \
  --value "rediss://$($tf output -raw redis_address):6379"
```

## 監視と予算

インフラのアラームと AWS Budgets の予算は prod にだけ置き、通知は SNS トピック
`anontopic-prod-alerts` からメールで届ける（ADR-0002、ADR-0033）。dev には置かない。

通知先のアドレスは prod の `terraform.tfvars` の `alarm_email` に書く。CI の plan と揃えるため、
GitHub のシークレット `ALARM_EMAIL` にも同じ値を登録する。初回の apply の後に AWS から届く
確認メールのリンクを開くまで、通知は届かない。

| アラーム | 条件 |
| --- | --- |
| `anontopic-prod-api-cpu` | API のサービスの CPU 使用率の平均が 80% を 15 分超える |
| `anontopic-prod-api-memory` | API のサービスのメモリ使用率の最大が 85% を超える |
| `anontopic-prod-api-unhealthy-targets` | ヘルスチェックに失敗しているタスクが 3 分続けてある |
| `anontopic-prod-api-5xx` | ALB と API のタスクの 5xx が 5 分間で 10 件を超える |
| `anontopic-prod-web-cpu` | フロントエンドのサービスの CPU 使用率の平均が 80% を 15 分超える |
| `anontopic-prod-web-memory` | フロントエンドのサービスのメモリ使用率の最大が 85% を超える |
| `anontopic-prod-web-no-healthy-targets` | ヘルスチェックに通るフロントエンドのタスクが 3 分続けて無い |
| `anontopic-prod-web-5xx` | フロントエンドのタスクの 5xx が 5 分間で 10 件を超える |
| `anontopic-prod-db-cpu` | RDS の CPU 使用率の平均が 80% を 15 分超える |
| `anontopic-prod-db-connections` | RDS への接続数が 50 を超える |
| `anontopic-prod-redis-memory` | Redis のメモリ使用率が 80% を超える |
| `anontopic-prod-nat-status-check` | NAT インスタンスのステータスチェックが 2 分続けて失敗する |

`api` の ALB の 2 つと `web` の 4 つは、`domain_name` を設定してから作られる。ALB 自身が返す
5xx はターゲットグループごとに分かれないため、フロントエンドへの要求の分も `api-5xx` に入る。

予算 `anontopic-account-monthly` はアカウント全体（dev と state バケットを含む）の月額を見て、
実際の費用が $270 の 50% / 80% / 100% を超えたら通知する。費用のデータは 1 日に数回しか
更新されないため、通知は数時間遅れることがある。超えてもサービスは自動では止まらない。

RDS の Database Insights は Standard モードの 7 日保持で、費用はかからない。CloudWatch の
コンソールの Database Insights から、SQL ごとの負荷を見られる。

## プロバイダのバージョン

プロバイダのバージョンは各ルートモジュールの `.terraform.lock.hcl` で固定している。
上げるときは lock ファイルも作り直してコミットする。

```bash
terraform -chdir=infra/environments/dev init -upgrade -backend=false
terraform -chdir=infra/environments/dev providers lock \
  -platform=linux_amd64 -platform=darwin_arm64 -platform=darwin_amd64
```
