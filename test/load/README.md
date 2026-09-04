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

Vegeta は HTTP のみを対象とする。WebSocket（`/ws/rooms/{roomID}`）の負荷試験には別の手段が必要。

## 同時接続の上限を確かめる

同時接続数は Redis 上の期限付きの席で数える。上限に達すると、ハンドシェイクは
セッションを読む前に 503 を返す。理由は
[ADR-0013](../../docs/adr/0013-cap-connections-with-leases-in-redis.md) にある。

1,000 本つながずに上限の挙動を見るには、上限を下げて起動する。

```bash
CAPACITY_MAX_CONNECTIONS=5 make up   # compose 経由。make backend-run でも同じ変数が効く
```

数えている席は Redis から直接読める。試験中にこの値が上限を超えないことが、
上限が守られている証拠になる。

```bash
docker compose exec redis redis-cli ZCARD capacity:connections
docker compose exec redis redis-cli --stat   # 試験中に張り付けて増減を見る
```

席は接続が閉じるときに返り、返らなかった席も `CAPACITY_LEASE_TTL`（既定 30 秒）で空く。
サーバーを `kill -9` してから 30 秒待つと `ZCARD` が 0 に戻ることで、異常終了しても
数が減らないまま詰まらないことを確認できる。

接続数を数える仕組み自体は `go test ./internal/capacity/` が Redis に対して検証する
（Redis が無い環境ではスキップされる）。上限ぴったりまでしか通さないことは、
`TestRedisStoreNeverCountsPastTheLimit` が同時要求で確かめている。
