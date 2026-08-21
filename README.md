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
docker compose up -d   # 初回はイメージのビルドで数分かかる
docker compose ps      # 全サービスが healthy になれば準備完了
```

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

### 停止

```bash
docker compose down      # コンテナのみ削除（DB / Redis のデータは残る）
docker compose down -v   # データボリュームごと削除（初期状態に戻る）
```

ポートが既に使われている場合は `.env` で `POSTGRES_PORT` / `REDIS_PORT` / `API_PORT` /
`WEB_PORT` を変更する。コンテナ間の通信はサービス名で行うため影響しない。

## セットアップ（コンテナを使わない場合）

```bash
go mod download
cd web && npm install
```

Go サーバーを `make run` で直接起動する場合も、PostgreSQL と Redis は
`docker compose up -d postgres redis` で用意する。接続先の既定値は
`.env.example` のポートに合わせてあり、`DATABASE_URL` / `REDIS_URL` で上書きできる。

## よく使うコマンド

```bash
make build       # go build ./...
make run         # ローカルで API サーバーを起動（http://localhost:8080/healthz）
make lint        # golangci-lint
make web-build   # Next.js のビルド
make web-lint    # ESLint
make check       # CI と同じ一連のチェック
```

`make help` で全ターゲットを確認できる。

## 負荷テスト

[Vegeta](https://github.com/tsenart/vegeta) を `go.mod` の tool 依存として管理しているため、
別途インストールする必要はない。

```bash
make run                                      # 別ターミナルでサーバーを起動
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
