# anontopic

Anonymous topic-based random chat service (2-3 person rooms) built with Go backend and Next.js frontend.

## 構成

Goのモジュラーモノリス（バックエンド）とNext.js（フロントエンド）を1つのリポジトリで管理する。

```
.
├── cmd/server/          # HTTP / WebSocketサーバーのエントリポイント（唯一の配線場所）
├── internal/
│   ├── matching/        # 待機キューと2〜3人ルームの成立ロジック
│   ├── chat/            # ルーム内のリアルタイムメッセージング（WebSocket）
│   ├── topic/           # トピックカタログとその公開状態
│   ├── moderation/      # 自動コンテンツチェックと制裁アクション
│   ├── report/          # 通報の受付とレビューフロー
│   └── retention/       # 保持期間を過ぎたデータの削除
├── web/                 # Next.js（App Router / TypeScript / Tailwind CSS）
├── infra/               # Terraform（M3で使用）
└── db/
    ├── migrations/      # スキーマ変更
    └── seeds/           # 初期データ
```

## 必要なツール

| ツール | バージョン |
| --- | --- |
| Go | 1.26以上 |
| Node.js | 22以上 |
| golangci-lint | 2系 |

## ローカル環境（Docker Compose）

PostgreSQL / Redis / API / Webの4サービスをDocker Composeで起動する。

```bash
cp .env.example .env   # ポートなどを変えたい場合のみ編集する
make up                # 初回はイメージのビルドで数分かかる
```

`make up` は全サービスがhealthyになるのを待ってから、マイグレーションとシードを流す。
クローン直後でもトピックが入った状態で開発を始められる。

| サービス | 用途 | ホスト側 |
| --- | --- | --- |
| `api` | Goサーバー（airによるホットリロード） | http://localhost:8080 |
| `web` | Next.js dev server | http://localhost:3000 |
| `postgres` | PostgreSQL 18 | localhost:5432 |
| `redis` | Redis 8 | localhost:6379 |

`api` / `web` はソースをバインドマウントしているため、ホスト側でファイルを編集すると
そのまま反映される。

### 疎通確認

```bash
curl localhost:8080/healthz   # プロセスが生きているか
curl localhost:8080/readyz    # PostgreSQLとRedisに接続できるか
```

`/readyz` はどちらかに接続できないと503と失敗した接続先を返す。

### セッション

利用者は会員登録せず、匿名のセッショントークンで識別する。トークンはHttpOnly Cookie
（`anontopic_session`）で配り、実体はRedisに置く。方式の理由は
[ADR-0005](docs/adr/0005-anonymous-session-tokens-in-redis.md) にある。

```bash
curl -i -X POST localhost:8080/api/session -c cookie.txt   # 発行（既存が有効なら延長）
curl -i -X DELETE localhost:8080/api/session -b cookie.txt # 明示的な離脱で失効
```

`/ws/rooms/{roomID}` は有効なセッションが無いと401を返す。

### トピック

トピック選択画面は `GET /api/topics` の結果だけで描画する。返るのは `is_active` が
trueのトピックだけで、APIサーバーはこれをプロセス内に一定時間キャッシュする。
理由は [ADR-0006](docs/adr/0006-cache-the-topic-list-in-process.md) にある。

```bash
curl localhost:8080/api/topics
```

追加・改名・公開停止は管理APIから行う。`ADMIN_API_TOKEN` を設定した環境でだけ
生えるエンドポイントで、設定していなければ404を返す。ローカルで使うには
`cp .env.example .env` して値を入れてから `make up` する。認証方式の背景は
[ADR-0007](docs/adr/0007-guard-the-admin-api-with-a-static-bearer-token.md) にある。

```bash
TOKEN=local-development-admin-token   # .envのADMIN_API_TOKENと同じ値
curl -H "Authorization: Bearer $TOKEN" localhost:8080/api/admin/topics
curl -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"ゲーム"}' localhost:8080/api/admin/topics
curl -X PATCH -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"is_active":false}' localhost:8080/api/admin/topics/1
```

会話から参照されているトピックは削除できず、409を返す。選択画面から消すには
`is_active` をfalseにする。

### マッチング

トピックとルーム種別（2人 / 3人）ごとの待機キューに入り、人数が揃った時点で会話が
成立する。キューはRedisに置き、取り出しはLuaスクリプトで原子的に行うため、
サーバーが複数台でも1人が2つのルームに入ることはない。設計の理由は
[ADR-0008](docs/adr/0008-hold-the-matching-queue-in-a-redis-sorted-set.md) にある。

```bash
curl -X POST localhost:8080/api/session -c cookie.txt   # 先にセッションが要る
curl -X POST -b cookie.txt -H 'Content-Type: application/json' \
  -d '{"topic_id":1,"room_type":2}' localhost:8080/api/matching   # 待機に入る
curl -b cookie.txt localhost:8080/api/matching                     # 状態を問い合わせる
curl -X DELETE -b cookie.txt localhost:8080/api/matching           # 待機をやめる
```

`POST` は成立すれば200、待機のままなら202を返す。待機中のクライアントは `GET` を
繰り返し呼び、`state` が `matched` になったところで返ってきた会話IDを使って
`/ws/rooms/{roomID}` につなぐ。

3人ルームは3人揃うのを待つが、先頭の利用者が `MATCHING_FALLBACK_AFTER` を超えて
待っている場合は2人で成立し、`room_type` に2が返る。理由は
[ADR-0009](docs/adr/0009-fall-back-to-a-two-person-room.md) にある。

### チャット

成立した会話には `/ws/rooms/{roomID}` にWebSocketでつなぐ。`roomID` はマッチングが返した
会話IDで、接続はセッションCookieで認証する。Originが `APP_ALLOWED_ORIGINS` にも
自ホストにも一致しない接続は、セッションに触れる前に拒否する。

メッセージはRedis Pub/Subを通してサーバー間で配送するため、参加者がそれぞれ別のサーバーに
つないでいても届く。参加者はセッショントークンではなく会話ごとの番号（1起点）で表し、
トークンが他の参加者に渡ることはない。

送信前にモデレーションの判定を挟む口を用意してあり、ブロックされたメッセージは相手に届かず、
送信者にだけ `{"type":"error","code":"blocked"}` が返る。判定を行うmoderationモジュールは
まだ配線していないため、現状はすべてのメッセージがそのまま配送される（起動時に警告を出す）。

片方が切断すると、残った参加者にはすぐ退出が伝わる。接続中の参加者が1人以下のまま
`CHAT_REJOIN_GRACE` を過ぎると会話が終了し、`conversations.ended_at` と `end_reason` に
記録される。猶予の間は同じ会話に再接続でき、参加者が増えることはない。理由は
[ADR-0011](docs/adr/0011-end-a-conversation-after-a-rejoin-grace.md)、会話のテーブルを
どのモジュールが書くかは
[ADR-0010](docs/adr/0010-split-the-conversation-tables-by-lifecycle-phase.md) にある。

やり取りするフレームの形式は [docs/openapi.yaml](docs/openapi.yaml) に書いてある。

### 接続数の上限とレート制限

コストの上振れを止めるため、同時接続数の上限をアプリケーション層で強制する（設計書6）。
接続はRedis上にリースを1つ取り、開いている間そのリースを更新し続け、閉じるときに返す。
異常終了したサーバーのリースは更新が止まった時点から `CAPACITY_LEASE_TTL` で切れるため、
数が減らないまま詰まることがない。理由は
[ADR-0013](docs/adr/0013-cap-connections-with-leases-in-redis.md) にある。

| 制限 | 単位 | 超えたときの応答 |
| --- | --- | --- |
| 同時接続数（`CAPACITY_MAX_CONNECTIONS`） | 全サーバー合計 | ハンドシェイクに503と `Retry-After` |
| 1アドレスの接続数（`CAPACITY_MAX_CONNECTIONS_PER_IP`） | IPハッシュ | ハンドシェイクに429と `Retry-After` |
| メッセージ送信（`CAPACITY_MESSAGE_*`） | セッショントークン | `{"type":"error","code":"rate_limited"}` |
| マッチング要求（`CAPACITY_MATCH_*`） | IPハッシュ | `POST /api/matching` に429と `Retry-After` |

拒否は接続を数える段で返るため、上限に達している間の接続はセッションの参照も会話の参照も
行わない。手元で確かめるには `CAPACITY_MAX_CONNECTIONS` を小さくして起動する。

エンドポイントの仕様は [docs/openapi.yaml](docs/openapi.yaml) にある。手元で読むには
`npx @redocly/cli preview-docs docs/openapi.yaml`、検査するには
`npx @redocly/cli lint docs/openapi.yaml` を使う。

APIサーバーの設定は環境変数で行う。

| 変数 | 既定値 | 用途 |
| --- | --- | --- |
| `APP_ALLOWED_ORIGINS` | `http://localhost:3000` | Cookie付きリクエストとWebSocket接続を許可するオリジン（カンマ区切り） |
| `APP_TRUST_FORWARDED_FOR` | `false` | `X-Forwarded-For` の末尾を接続元とみなす。ロードバランサ経由でのみ有効にする |
| `SESSION_COOKIE_SECURE` | `true` | Cookieに `Secure` を付ける。httpのローカルでは `false` |
| `SESSION_COOKIE_SAMESITE` | `lax` | `lax` / `strict` / `none`。`none` は `SESSION_COOKIE_SECURE=true` が必要 |
| `SESSION_IP_HASH_SECRET` | プロセス起動ごとの乱数 | IPハッシュの鍵。未設定だと再起動でハッシュが変わり、ハッシュに紐づくBANが外れる |
| `ADMIN_API_TOKEN` | なし | トピック管理APIが要求するBearerトークン。未設定だと管理エンドポイントを登録しない |
| `TOPIC_CACHE_TTL` | `5m` | トピック一覧をプロセス内に保持する時間。`300s` のようなGoのduration表記 |
| `MATCHING_WAIT_TTL` | `5m` | 待機キューに並び続けられる時間。超えた利用者はキューから外れる |
| `MATCHING_FALLBACK_AFTER` | `60s` | 3人ルームの待機がこの時間を超えたら2人で成立させる |
| `CHAT_REJOIN_GRACE` | `30s` | 切断した参加者を待つ時間。接続中が1人以下のままこの時間を超えると会話を終了する |
| `CAPACITY_MAX_CONNECTIONS` | `1000` | 全サーバー合計で同時に持つWebSocket接続の上限。達している間は新規接続を503で拒否する |
| `CAPACITY_MAX_CONNECTIONS_PER_IP` | `5` | 1つのIPハッシュが同時に持てる接続数。超えた接続は429で拒否する |
| `CAPACITY_LEASE_TTL` | `30s` | 接続がリースを持ち続ける時間。更新が止まったリースはこの時間で切れ、数から外れる |
| `CAPACITY_RENEW_INTERVAL` | `10s` | 開いている接続がリースを更新する間隔。`CAPACITY_LEASE_TTL` の1/3に収まらない値は起動時に切り詰める |
| `CAPACITY_MESSAGE_BURST` | `5` | 続けて送れるメッセージ数 |
| `CAPACITY_MESSAGE_INTERVAL` | `1s` | 送信枠が1通ずつ回復する間隔 |
| `CAPACITY_MATCH_BURST` | `3` | 続けて出せるマッチング要求の数。IPハッシュ単位で数える |
| `CAPACITY_MATCH_INTERVAL` | `10s` | マッチング要求の枠が1回ずつ回復する間隔 |

### フロントエンド

`web/` のNext.jsアプリは、経路ごとに描画方法を分けている。理由は
[ADR-0014](docs/adr/0014-render-the-marketing-pages-statically-and-the-topic-pages-per-request.md)
にある。

| 経路 | 描画 | 内容 |
| --- | --- | --- |
| `/` | 静的生成 | LP。サービスの説明とトピック選択への導線 |
| `/about` | 静的生成 | 紹介ページ。使い方、ルーム種別、禁止事項、よくある質問 |
| `/topics` | リクエストごと | トピックとルーム種別の選択。`GET /api/topics` の結果をサーバーで埋める |
| `/waiting` | リクエストごと | 待機画面。`GET /api/matching` を2秒ごとに読んで成立を待つ |

`sitemap.xml` / `robots.txt` / OGP画像 / faviconは `web/app/` のNext.jsの
ファイル規約で生成する。`/waiting` はセッションを持つ人にしか意味がないため、
`robots.txt` でクロール対象から外している。

フロントエンドの設定は環境変数で行う。`NEXT_PUBLIC_` が付く値はブラウザに配られる。

| 変数 | 既定値 | 用途 |
| --- | --- | --- |
| `API_BASE_URL` | composeでは `http://api:8080` | Next.jsのサーバーから見たAPIのオリジン |
| `NEXT_PUBLIC_API_BASE_URL` | `http://localhost:8080` | ブラウザから見たAPIのオリジン |
| `NEXT_PUBLIC_WS_BASE_URL` | `ws://localhost:8080` | ブラウザから見たWebSocketのオリジン |
| `NEXT_PUBLIC_SITE_URL` | `http://localhost:3000` | 公開時のオリジン。OGP・canonical・`sitemap.xml` の絶対URLに使う |

`WEB_PORT` を変えたときは、`NEXT_PUBLIC_SITE_URL` とAPI側の `APP_ALLOWED_ORIGINS` も
同じポートに合わせる。合っていないと、ブラウザからのセッション発行がCORSで止まる。

### スキーマとデータ

| | migrate | seed |
| --- | --- | --- |
| 対象 | スキーマ（テーブル・インデックス） | データ（行） |
| 実行回数 | 各バージョン1回だけ | 何度でも（冪等） |
| 適用済み管理 | `schema_migrations` テーブル | 追跡しない |
| 環境差 | 全環境で同じ | 環境ごとに違う |

- マイグレーションは `db/migrations/`。`cmd/migrate` がembedして適用する。
  `go run ./cmd/migrate down` で1つ戻せる。
- シードは `db/seeds/`。`base/` は全環境、`dev/` は `APP_ENV=production` 以外でのみ入る。
  `dev/` には通報済みの会話や90日を超えた会話が含まれ、通報一覧や削除バッチの確認に使う。
- `messages` は月次パーティション。初期マイグレーションが当月の前後10か月分を作る。
- テーブルとカラムの論理名は `COMMENT ON` でスキーマ自身に持たせる。別ファイルの定義書に
  すると実装とずれるため、テーブルを追加するときは論理名も同じマイグレーションに含める。
  `psql` で `\d+ topics` を実行すると確認できる。

### 停止

```bash
make down    # コンテナのみ削除（DB / Redisのデータは残る）
make downd   # データボリュームごと削除（初期状態に戻る）
make reset   # downdしてからup（初期状態で作り直す）
```

ポートが既に使われている場合は `.env` で `POSTGRES_PORT` / `REDIS_PORT` / `API_PORT` /
`WEB_PORT` を変更する。コンテナ間の通信はサービス名で行うため影響しない。

## 開発ツール

golangci-lintとgitleaksを、リポジトリが固定しているバージョンで入れる。

```bash
make tools                        # 両方
make tools TOOLS=golangci-lint    # 片方だけ
```

バージョンはMakefileの `GOLANGCI_LINT_VERSION` / `GITLEAKS_VERSION` が唯一の定義で、
CIも同じ値を使う。`go install` の出力先（`$(go env GOPATH)/bin`）がPATHに無い場合や、
別の場所にある同名の実行ファイルが優先される場合は警告が出る。

## git hooks

コミット前の検査を有効にする。クローンした直後は無効なので、最初に1度実行する。

```bash
make hooks
```

`.githooks/pre-commit` が見るのは、ステージした内容だけで完結する軽い検査に限っている。
テストとビルドはCIの担当。

| 検査 | 動作 |
| --- | --- |
| シークレットの混入（gitleaks） | コミットを止める。gitleaksが無い環境では飛ばす |
| gofmt / Prettier | コミットを止める |
| コメント規約（タスク番号・過去の実装との比較） | 知らせるだけで止めない。正当な用法にも当たるため |
| マイグレーションの番号重複・downの欠落 | コミットを止める |

## セットアップ（コンテナを使わない場合）

```bash
go mod download
cd web && npm install
```

Goサーバーを `make backend-run` で直接起動する場合も、PostgreSQLとRedisは
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

make build         # backend-buildとfrontend-build
make test          # backend-testとfrontend-test
make lint          # backend-lintとfrontend-lint
make check         # push前に通すべき一連のチェック

make backend-run   # APIサーバーだけを直接起動
make frontend-lint # ESLintだけを実行
```

`make help` で全ターゲットを確認できる。

## 負荷テスト

[Vegeta](https://github.com/tsenart/vegeta) を `go.mod` のtool依存として管理しているため、
別途インストールする必要はない。

```bash
make backend-run                              # 別ターミナルでサーバーを起動
make load-test                                # 既定: 50 req/sを30秒
make load-test LOAD_RATE=200 LOAD_DURATION=1m # レート・時間の上書き
make load-report                              # 直近の結果をレポート＋レイテンシ分布で表示
```

詳細は [test/load/README.md](./test/load/README.md) を参照。

## モジュール間の依存ルール

`internal/` 配下の各モジュールは独立している。**他モジュールのDBモデル / リポジトリを直接importしてはならない。**
詳細は [CONTRIBUTING.md](./CONTRIBUTING.md) を参照。

## コメントの書き方

コードコメントでは、過去の実装との比較（従来 / 以前 / 旧〜 / 置換 / 〜と同じなど）を書かず、
定義のない造語も使わない。詳細は [CONTRIBUTING.md](./CONTRIBUTING.md#コメントの書き方) を参照。
