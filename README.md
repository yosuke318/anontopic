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

## セットアップ

```bash
go mod download
cd web && npm install
```

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
