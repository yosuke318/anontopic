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
| `terraform.tfvars` | 変数の値（`aws_account_id` など） | `example.tfvars` |
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
make infra-plan ENV=dev
```

prod も同じ手順で `ENV=prod` にする。

## 日々の作業

```bash
make infra-fmt                # 整形
make infra-check              # fmt の検査と validate（AWS の認証情報は要らない）
make infra-plan ENV=dev       # リモートの state に対して plan を取る
```

プロバイダのバージョンは各ルートモジュールの `.terraform.lock.hcl` で固定している。
上げるときは lock ファイルも作り直してコミットする。

```bash
terraform -chdir=infra/environments/dev init -upgrade -backend=false
terraform -chdir=infra/environments/dev providers lock \
  -platform=linux_amd64 -platform=darwin_arm64 -platform=darwin_amd64
```
