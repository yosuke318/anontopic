# 負荷テスト

[Vegeta](https://github.com/tsenart/vegeta) を Go の tool 依存として `go.mod` に入れているため、
個別のインストールは不要。

```bash
make run                    # 別ターミナルでサーバーを起動
make load-test              # 既定: 50 req/s を 30 秒
make load-test LOAD_RATE=200 LOAD_DURATION=1m
```

- 攻撃対象は `test/load/targets.txt`（Vegeta の targets 形式）で定義する。
- 結果の生データは `test/load/results/` に出力され、Git 管理外。
  `make load-report` で直近の結果からレポートとレイテンシ分布を再表示できる。

Go コードから使う場合は `github.com/tsenart/vegeta/v12/lib` を import する
（`go.mod` に入っているので追加の `go get` は不要）。

Vegeta は HTTP のみを対象とする。WebSocket（`/ws/rooms/{roomID}`）を含む試験は次の
「チャットのシナリオ」を使う。

## チャットのシナリオ

`test/load/chat` は、利用者と同じ手順を仮想ユーザーの数だけ並べて流す。

1. `POST /api/session` でセッションを受け取る
2. `POST /api/matching` で待機キューに入り、成立するまで `GET /api/matching` を 1 秒ごとに呼ぶ
3. `/ws/rooms/{roomID}` につなぎ、`-interval` ごとにメッセージを送る
4. `-stay` が過ぎたら接続を閉じ、`DELETE /api/matching` と `DELETE /api/session` で退出する

続けて開始したユーザーは同じキューに入るため、互いにマッチングする。

```bash
make load-chat                                   # ローカルで 4 人、1 分
make load-chat CHAT_BASE=https://api.dev.example.com CHAT_USERS=1000 CHAT_STAY=10m
go run ./test/load/chat -h                       # すべてのフラグ
```

終わると、各段階を通った人数・失敗の内訳・遅延の分布を出す。

| 項目 | 意味 |
| --- | --- |
| `matched` / `session_ok` | その段階を通った仮想ユーザーの数 |
| `websocket_503` | 全体の接続数の上限で断られた数。上限を超えて流したときに、超えた分だけ出れば上限が効いている |
| `websocket_429` / `matching_rate_limited` | IP アドレスごとの上限に当たった数 |
| `error_*` / `ended_*` | サーバーが返した `error` の `code` と、`ended` の `reason` |
| `delivery` | メッセージを送ってから、同じルームの他の参加者が受け取るまでの時間 |
| `handshake` | WebSocket のハンドシェイクにかかった時間 |

仮想ユーザーはすべて 1 つのプロセスで動くため、`delivery` は同じ時計で測っている。
本文は `負荷試験 <英字の通し番号> あああ…` の形にする。数字が続くと電話番号として
ブロックされるため、通し番号は英字で書く。

### 1 台のクライアントから流すときの上限

サーバーは IP アドレスごとに、接続数（`CAPACITY_MAX_CONNECTIONS_PER_IP`、既定 5）と
マッチングの要求数（`CAPACITY_MATCH_BURST`、既定 3 回）を絞る。1 台から流すと、
ここで止まって全体の上限まで届かない。

- ローカル: `CAPACITY_MAX_CONNECTIONS_PER_IP=10000 CAPACITY_MATCH_BURST=10000 make up`
- dev: `terraform.tfvars` に `load_test = true` を書いて apply する。IP アドレスごとの
  上限を外し、API を 2 タスク、RDS を `db.t4g.small` にして prod に揃える。
  試験が終わったら外して apply するか、destroy する。

macOS はプロセスが開けるファイルの数の既定が 256 で、数百本つなぐと足りなくなる。
流す前に同じシェルで上げておく。

```bash
ulimit -n 8192
```

## 同時接続の上限を確かめる

同時接続数は Redis 上のリースで数える。上限に達すると、ハンドシェイクは
セッションを読む前に 503 を返す。理由は
[ADR-0013](../../docs/adr/0013-cap-connections-with-leases-in-redis.md) にある。

1,000 本つながずに上限の挙動を見るには、上限を下げて起動する。

```bash
CAPACITY_MAX_CONNECTIONS=5 make up   # compose 経由。make backend-run でも同じ変数が効く
```

数えているリースは Redis から直接読める。試験中にこの値が上限を超えないことが、
上限が守られている証拠になる。

```bash
docker compose exec redis redis-cli ZCARD capacity:connections
docker compose exec redis redis-cli --stat   # 試験中に張り付けて増減を見る
```

リースは接続が閉じるときに返り、返らなかったリースも `CAPACITY_LEASE_TTL`（既定 30 秒）で切れる。
サーバーを `kill -9` してから 30 秒待つと `ZCARD` が 0 に戻ることで、異常終了しても
数が減らないまま詰まらないことを確認できる。

接続数を数える仕組み自体は `go test ./internal/capacity/` が Redis に対して検証する
（Redis が無い環境ではスキップされる）。上限ぴったりまでしか通さないことは、
`TestRedisStoreNeverCountsPastTheLimit` が同時要求で確かめている。
