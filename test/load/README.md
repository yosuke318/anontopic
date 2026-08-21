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
