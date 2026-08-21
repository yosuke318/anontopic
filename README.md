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
