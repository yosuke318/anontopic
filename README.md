# anontopic

Anonymous topic-based random chat service (2-3 person rooms) built with Go backend and Next.js frontend.

## 構成

Go のモジュラーモノリス（バックエンド）と Next.js（フロントエンド）を 1 つのリポジトリで管理する。

```
.
├── cmd/server/          # HTTP / WebSocket サーバーのエントリポイント（唯一の配線場所）
├── internal/
│   ├── matching/        # 待機キューと 2〜3 人ルームの成立ロジック
│   ├── chat/            # ルーム内のリアルタイムメッセージング（WebSocket）
│   ├── topic/           # トピックカタログとその公開状態
│   ├── moderation/      # 自動コンテンツチェックと制裁アクション
│   ├── report/          # 通報の受付とレビューフロー
│   └── retention/       # 保持期間を過ぎたデータの削除
├── web/                 # Next.js（App Router / TypeScript / Tailwind CSS）
├── infra/               # Terraform（M3 で使用）
└── db/
    ├── migrations/      # スキーマ変更
    └── seeds/           # 初期データ
```

## 必要なツール

| ツール | バージョン |
| --- | --- |
| Go | 1.26 以上 |
| Node.js | 22 以上 |
| golangci-lint | 2 系 |

## ローカル環境（Docker Compose）

PostgreSQL / Redis / API / Web の 4 サービスを Docker Compose で起動する。

```bash
cp .env.example .env   # ポートなどを変えたい場合のみ編集する
make up                # 初回はイメージのビルドで数分かかる
```

`make up` は全サービスが healthy になるのを待ってから、マイグレーションとシードを流す。
クローン直後でもトピックが入った状態で開発を始められる。

| サービス | 用途 | ホスト側 |
| --- | --- | --- |
| `api` | Go サーバー（air によるホットリロード） | http://localhost:8080 |
| `web` | Next.js dev server | http://localhost:3000 |
| `postgres` | PostgreSQL 18 | localhost:5432 |
| `redis` | Redis 8 | localhost:6379 |

`api` / `web` はソースをバインドマウントしているため、ホスト側でファイルを編集すると
そのまま反映される。

### 疎通確認

```bash
curl localhost:8080/healthz   # プロセスが生きているか
curl localhost:8080/readyz    # PostgreSQL と Redis に接続できるか
```

`/readyz` はどちらかに接続できないと 503 と失敗した接続先を返す。

### セッション

利用者は会員登録せず、匿名のセッショントークンで識別する。トークンは HttpOnly Cookie
（`anontopic_session`）で配り、実体は Redis に置く。方式の理由は
[ADR-0005](docs/adr/0005-anonymous-session-tokens-in-redis.md) にある。

```bash
curl -i -X POST localhost:8080/api/session -c cookie.txt   # 発行（既存が有効なら延長）
curl -i -X DELETE localhost:8080/api/session -b cookie.txt # 明示的な離脱で失効
```

`/ws/rooms/{roomID}` は有効なセッションが無いと 401 を返す。

### トピック

トピック選択画面は `GET /api/topics` の結果だけで描画する。返るのは `is_active` が
true のトピックだけで、API サーバーはこれをプロセス内に一定時間キャッシュする。
理由は [ADR-0006](docs/adr/0006-cache-the-topic-list-in-process.md) にある。

```bash
curl localhost:8080/api/topics
```

追加・改名・公開停止は管理 API から行う。`ADMIN_API_TOKEN` を設定した環境でだけ
生えるエンドポイントで、設定していなければ 404 を返す。ローカルで使うには
`cp .env.example .env` して値を入れてから `make up` する。認証方式の背景は
[ADR-0007](docs/adr/0007-guard-the-admin-api-with-a-static-bearer-token.md) にある。

```bash
TOKEN=local-development-admin-token   # .env の ADMIN_API_TOKEN と同じ値
curl -H "Authorization: Bearer $TOKEN" localhost:8080/api/admin/topics
curl -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"ゲーム"}' localhost:8080/api/admin/topics
curl -X PATCH -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"is_active":false}' localhost:8080/api/admin/topics/1
```

会話から参照されているトピックは削除できず、409 を返す。選択画面から消すには
`is_active` を false にする。

エンドポイントの仕様は [docs/openapi.yaml](docs/openapi.yaml) にある。手元で読むには
`npx @redocly/cli preview-docs docs/openapi.yaml`、検査するには
`npx @redocly/cli lint docs/openapi.yaml` を使う。

API サーバーの設定は環境変数で行う。

| 変数 | 既定値 | 用途 |
| --- | --- | --- |
| `APP_ALLOWED_ORIGINS` | `http://localhost:3000` | Cookie 付きリクエストと WebSocket 接続を許可するオリジン（カンマ区切り） |
| `APP_TRUST_FORWARDED_FOR` | `false` | `X-Forwarded-For` の末尾を接続元とみなす。ロードバランサ経由でのみ有効にする |
| `SESSION_COOKIE_SECURE` | `true` | Cookie に `Secure` を付ける。http のローカルでは `false` |
| `SESSION_COOKIE_SAMESITE` | `lax` | `lax` / `strict` / `none`。`none` は `SESSION_COOKIE_SECURE=true` が必要 |
| `SESSION_IP_HASH_SECRET` | プロセス起動ごとの乱数 | IP ハッシュの鍵。未設定だと再起動でハッシュが変わり、ハッシュに紐づく BAN が外れる |
| `ADMIN_API_TOKEN` | なし | トピック管理 API が要求する Bearer トークン。未設定だと管理エンドポイントを登録しない |
| `TOPIC_CACHE_TTL` | `5m` | トピック一覧をプロセス内に保持する時間。`300s` のような Go の duration 表記 |

### スキーマとデータ

| | migrate | seed |
| --- | --- | --- |
| 対象 | スキーマ（テーブル・インデックス） | データ（行） |
| 実行回数 | 各バージョン 1 回だけ | 何度でも（冪等） |
| 適用済み管理 | `schema_migrations` テーブル | 追跡しない |
| 環境差 | 全環境で同じ | 環境ごとに違う |

- マイグレーションは `db/migrations/`。`cmd/migrate` が embed して適用する。
  `go run ./cmd/migrate down` で 1 つ戻せる。
- シードは `db/seeds/`。`base/` は全環境、`dev/` は `APP_ENV=production` 以外でのみ入る。
  `dev/` には通報済みの会話や 90 日を超えた会話が含まれ、通報一覧や削除バッチの確認に使う。
- `messages` は月次パーティション。初期マイグレーションが当月の前後 10 か月分を作る。
- テーブルとカラムの論理名は `COMMENT ON` でスキーマ自身に持たせる。別ファイルの定義書に
  すると実装とずれるため、テーブルを追加するときは論理名も同じマイグレーションに含める。
  `psql` で `\d+ topics` を実行すると確認できる。

### 停止

```bash
make down    # コンテナのみ削除（DB / Redis のデータは残る）
make downd   # データボリュームごと削除（初期状態に戻る）
make reset   # downd してから up（初期状態で作り直す）
```

ポートが既に使われている場合は `.env` で `POSTGRES_PORT` / `REDIS_PORT` / `API_PORT` /
`WEB_PORT` を変更する。コンテナ間の通信はサービス名で行うため影響しない。

## 開発ツール

golangci-lint と gitleaks を、リポジトリが固定しているバージョンで入れる。

```bash
make tools                        # 両方
make tools TOOLS=golangci-lint    # 片方だけ
```

バージョンは Makefile の `GOLANGCI_LINT_VERSION` / `GITLEAKS_VERSION` が唯一の定義で、
CI も同じ値を使う。`go install` の出力先（`$(go env GOPATH)/bin`）が PATH に無い場合や、
別の場所にある同名の実行ファイルが優先される場合は警告が出る。

## git hooks

コミット前の検査を有効にする。クローンした直後は無効なので、最初に 1 度実行する。

```bash
make hooks
```

`.githooks/pre-commit` が見るのは、ステージした内容だけで完結する軽い検査に限っている。
テストとビルドは CI の担当。

| 検査 | 動作 |
| --- | --- |
| シークレットの混入（gitleaks） | コミットを止める。gitleaks が無い環境では飛ばす |
| gofmt / Prettier | コミットを止める |
| コメント規約（タスク番号・過去の実装との比較） | 知らせるだけで止めない。正当な用法にも当たるため |
| マイグレーションの番号重複・down の欠落 | コミットを止める |

## セットアップ（コンテナを使わない場合）

```bash
go mod download
cd web && npm install
```

Go サーバーを `make backend-run` で直接起動する場合も、PostgreSQL と Redis は
`docker compose up -d postgres redis` で用意する。接続先の既定値は
`.env.example` のポートに合わせてあり、`DATABASE_URL` / `REDIS_URL` で上書きできる。

## よく使うコマンド

ターゲット名は `<領域>-<動作>` に揃えている。領域を省いたものは両方を実行する。

```bash
make up            # 起動 → マイグレーション → シードまで
make down          # 停止（データは残す）
make downd         # 停止してデータも破棄
make migrate       # スキーマだけ最新にする
make seed          # データだけ入れ直す

make build         # backend-build と frontend-build
make test          # backend-test と frontend-test
make lint          # backend-lint と frontend-lint
make check         # push 前に通すべき一連のチェック

make backend-run   # API サーバーだけを直接起動
make frontend-lint # ESLint だけを実行
```

`make help` で全ターゲットを確認できる。

## 負荷テスト

[Vegeta](https://github.com/tsenart/vegeta) を `go.mod` の tool 依存として管理しているため、
別途インストールする必要はない。

```bash
make backend-run                              # 別ターミナルでサーバーを起動
make load-test                                # 既定: 50 req/s を 30 秒
make load-test LOAD_RATE=200 LOAD_DURATION=1m # レート・時間の上書き
make load-report                              # 直近の結果をレポート＋レイテンシ分布で表示
```

詳細は [test/load/README.md](./test/load/README.md) を参照。

## モジュール間の依存ルール

`internal/` 配下の各モジュールは独立している。**他モジュールの DB モデル / リポジトリを直接 import してはならない。**
詳細は [CONTRIBUTING.md](./CONTRIBUTING.md) を参照。

## コメントの書き方

コードコメントでは、過去の実装との比較（従来 / 以前 / 旧〜 / 置換 / 〜と同じ など）を書かず、
定義のない造語も使わない。詳細は [CONTRIBUTING.md](./CONTRIBUTING.md#コメントの書き方) を参照。
