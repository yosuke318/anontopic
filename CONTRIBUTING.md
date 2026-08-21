# CONTRIBUTING

## モジュール間の依存ルール

`internal/` 配下の各パッケージ（`matching`, `chat`, `topic`, `moderation`, `report`, `retention`）は、
モジュラーモノリスにおける独立したモジュールとして扱う。将来サービス分割する際のコストを抑えるため、
以下のルールを守ること。

### 禁止事項

1. **他モジュールの DB モデル / リポジトリを直接 import しない。**
   例: `chat` が `report` の `Report` 構造体や `reportRepository` を import するのは NG。
2. **他モジュールのテーブルに直接クエリを投げない。**
   自モジュールが所有するテーブルにのみアクセスする。
3. **`internal/` のモジュール同士で循環依存を作らない。**

### 許可されること

1. **ID による参照。** 他モジュールのリソースは UUID などの ID で参照する
   （例: `report` は `roomID` / `messageID` を保持するだけで、`chat` のモデルは持たない）。
2. **インターフェース経由の呼び出し。** 呼び出し側が必要な操作をインターフェースとして定義し、
   具体的な実装は `cmd/server/main.go` で注入する。

   ```go
   // internal/matching/matching.go（呼び出し側がインターフェースを所有する）
   type RoomCreator interface {
       CreateRoom(ctx context.Context, topicID string, userIDs []string) (string, error)
   }
   ```

3. **DTO の受け渡し。** モジュール境界をまたぐデータは、永続化モデルではなく
   境界用の構造体（DTO）で受け渡す。

### 配線は 1 か所に集約する

モジュールの依存関係の組み立ては `cmd/server/main.go` でのみ行う。
依存グラフを 1 ファイルで追えるようにするため、モジュール内で他モジュールの
コンストラクタを直接呼ばないこと。

## コーディング規約

### Go

- フォーマットは `gofmt` / `goimports`（ローカルパッケージは `github.com/yosuke318/anontopic` にグループ化）。
- Lint は `golangci-lint run ./...`（設定は `.golangci.yml`）。
- push 前に `make build && make test && make lint` が通ることを確認する。

### TypeScript / Next.js

- フォーマットは Prettier（`web/.prettierrc.json`）、Lint は ESLint（`web/eslint.config.mjs`）。
- push 前に `cd web && npm run lint && npm run typecheck && npm run build` が通ることを確認する。

まとめて確認する場合は `make check` を使う。

## ブランチ / コミット

- ブランチ名は Linear が生成するもの（例: `youman318/yosuke-115-...`）を使う。
- PR には対応する Linear の issue を紐付ける。
