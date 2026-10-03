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

ドメインを取得したら、同じ画面の Variables に `DOMAIN_NAME`（Route 53 のホストゾーン名）を
登録する。未登録の間、CI の plan は証明書・DNS レコード・HTTPS リスナーを作らない構成で取る。
ローカルの `terraform.tfvars` の `domain_name` と揃えないと、CI とローカルで plan が食い違う。

CI の `インフラ plan` ジョブは `anontopic-terraform-plan` ロールを OIDC で引き受けて、
dev と prod の plan を取る。ログには件数の行だけを出す。

`anontopic-terraform-apply` ロールは、GitHub の Environment `dev` / `prod` を指定したジョブから
だけ引き受けられる。apply のワークフローを足すときは、Environment の保護ルールで
デプロイできるブランチを `main` に絞り、prod には承認者を付ける。

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

terraform -chdir=infra/environments/dev apply
```

ECR のタグは上書きできない。2 回目以降のデプロイは別のタグでイメージを入れ、タスク定義の
新しいリビジョンを登録してサービスを更新する。Terraform はサービスが使うリビジョンを追わないため、
Terraform でタスク定義を変えたときも、次のデプロイまで動いているタスクには反映されない。

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

## プロバイダのバージョン

プロバイダのバージョンは各ルートモジュールの `.terraform.lock.hcl` で固定している。
上げるときは lock ファイルも作り直してコミットする。

```bash
terraform -chdir=infra/environments/dev init -upgrade -backend=false
terraform -chdir=infra/environments/dev providers lock \
  -platform=linux_amd64 -platform=darwin_arm64 -platform=darwin_amd64
```
