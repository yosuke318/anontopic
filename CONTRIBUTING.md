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
- push 前に `make backend-lint && make backend-test && make backend-build` が通ることを確認する。

### TypeScript / Next.js

- フォーマットは Prettier（`web/.prettierrc.json`）、Lint は ESLint（`web/eslint.config.mjs`）。
- push 前に `make frontend-lint && make frontend-build` と `cd web && npm run typecheck` が通ることを確認する。
- `next-env.d.ts` と `.next/types/` は `next typegen` が生成するファイルなので Git 管理外。
  `npm install`（postinstall）、`npm run typecheck`、`npm run build` がそれぞれ生成するため、
  通常は意識しなくてよい。クローン直後にエディタが `LayoutProps` 等を解決できない場合は
  `cd web && npx next typegen` を実行する。

まとめて確認する場合は `make check` を使う。

### コメントの書き方

Go / TypeScript を問わず、すべてのコードコメントに適用する。人が書く場合も AI に書かせる場合も同じ。

#### 1. 過去の実装と比較しない

コメントは「今のコードが何をするか」だけを説明する。実装の変遷は git log と PR に残るので、
コメントに書くと**実装が次に変わった瞬間に嘘になる**。レビュー時点の一時的な文脈が、
そのままコードベースに残り続けてしまう。

次のような表現は使わない。

- 日本語: 従来 / 今まで / これまで / 以前 / 旧〜 / 変更前 / 置換 / 〜に置き換えた / 従来通り / 〜と同じ / 新しく〜 / 修正後
- 英語: previously / formerly / used to / instead of / replaces / no longer / same as before / new implementation

```go
// NG: 差分の説明になっている
// 従来は sync.Mutex を使っていたが、ここでは sync.RWMutex に置換した。
// 旧 matchUsers と同じロジック。

// OK: 現在の仕様と理由の説明になっている
// 参加者の読み取りが書き込みより圧倒的に多いため RWMutex を使う。
```

「なぜこうなっているか」を書きたい場合は、過去の実装ではなく**制約や理由そのもの**を書く。

```go
// NG: 仕様変更前の挙動を根拠にしている
// 以前は 2 人でも成立させていたが、今は 3 人待つ。

// OK: 制約を根拠にしている
// 会話が続きにくいため 2 人ルームは最終手段とし、まず 3 人揃うのを待つ。
```

#### 2. タスク番号・チケット番号を書かない

`YOSUKE-123` / `#123` / `TODO(YOSUKE-115)` / 「issue 42 参照」のような参照はコメントに残さない。

issue は閉じるので、後から番号を見ても今のコードがなぜこうなっているかは分からない。
リポジトリの外への参照になるため、clone しただけでは解決できず、トラッカーを移行すれば
ただの文字列になる。コードと issue の対応は commit と PR に残っている。

未完成の実装は、コメントで予告するのではなく Linear の issue で追う。コードには
「今どう振る舞うか」だけを書く。

```go
// NG: 番号を見に行かないと何も分からない
// TODO(YOSUKE-115): implement room join, fan-out and disconnect handling.

// OK: 今の振る舞いを書く
// ルームへの参加・配送・切断はまだ実装しておらず、501 を返す。
```

リポジトリ内にある `docs/adr/` の ADR は指してよい。clone すれば読めて、閉じないため。

#### 3. 造語を作らない

コメントに登場する用語は、次のいずれかに限る。

- コード上の識別子（型名・関数名・変数名）
- 基本設計書 / Linear の issue で定義済みの用語
- 一般的な技術用語（WebSocket, リトライ, バックプレッシャー など）

AI に生成させたコメントには、このリポジトリのどこにも定義がない造語や、
それらしいだけの独自概念が紛れ込みやすい。**定義のない名詞を新しく登場させない。**

```go
// NG: 「マッチングプール」「セッションハーモナイザ」はコード上にも設計書にも存在しない
// マッチングプールからセッションハーモナイザ経由でルームを確定させる。

// OK: 実在する識別子で書く
// waitingQueue から取り出したユーザーを RoomCreator に渡してルームを確定させる。
```

新しい概念が本当に必要なら、まず基本設計書か CONTRIBUTING.md に用語として定義し、
可能ならコード上の型名・関数名にする。コメントだけに存在する用語は作らない。

#### 4. コメントを書かない選択も正しい

識別子を適切に命名すれば不要になるコメントは、コメントではなく命名で解決する。
残すのは、コードを読んでも分からない「なぜ」と、外部仕様・制約への参照。

## ブランチ / コミット

- ブランチ名は Linear が生成するもの（例: `youman318/yosuke-115-...`）を使う。
- PR には対応する Linear の issue を紐付ける。
