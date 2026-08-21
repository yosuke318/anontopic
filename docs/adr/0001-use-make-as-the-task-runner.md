# ADR-0001: ローカル開発のコマンドを Make に統一する

- ステータス: Accepted
- 日付: 2026-08-21
- 関連 issue: YOSUKE-116, YOSUKE-117

## 文脈

- 開発体制は 1 名。
- リポジトリには初期構築の時点から `Makefile` があり、`make check` が AGENTS.md と
  CONTRIBUTING.md の両方に「push 前に通すこと」として書かれている。CI もこれを使う前提。
- Docker Compose のローカル環境を入れたことで、環境の起動・停止コマンドを追加する必要が出た。
  YOSUKE-117 は当初 go-task で `task up / down / downd` を用意する内容だった。
- Go と Next.js が同居しているため、ビルド・テスト・lint がそれぞれ 2 系統ある。
  片方だけを走らせたい場面と、両方まとめて走らせたい場面の両方がある。

## 決定

Make に統一する。go-task は導入しない。

ターゲット名は `<領域>-<動作>` に揃える（`backend-test`, `frontend-lint`）。
領域を省いた `build` / `test` / `lint` / `fmt` は両方を実行する集約とする。
環境操作は `up` / `down` / `downd` / `reset` / `logs`。

## 検討した代替案

### 案A: go-task を導入して Taskfile.yml に寄せる

YAML で書けて、依存関係やクロスプラットフォームの扱いは Make より素直。
ただし `make check` を前提にした記述（AGENTS.md、CONTRIBUTING.md、CI）と既存ターゲットを
すべて書き換える必要があり、実行環境に追加インストールも要る。
1 名の開発で得られるものが「書きやすさ」に留まるため、移行コストに見合わない。

### 案B: Make と go-task の併用（Docker 操作だけ task）

コマンドの入口が 2 つになり、どちらで何をするのかを毎回判断することになる。
CI からも両方を呼ぶ必要が出る。

### 案C: ターゲット名をコロン区切りにする（`backend:test`）

名前空間として読みやすい。しかし GNU Make はコロンを含むターゲットを
**前提条件リストで解決できない**（3.81 で確認）。`build: backend\:build` と書くと
`No rule to make target` になり、`.PHONY` に並べた場合は何も実行しないまま成功したように見える。
集約ターゲットをサブ make（`$(MAKE) backend:test`）で書けば動くが、
書き方を誤ったときに黙って壊れるため採らない。

## 影響

- `web-build` / `web-lint` / `web-format` は `frontend-*` に変わる。
  Go 向けの無印 `build` / `test` / `lint` は `backend-*` に変わり、無印は集約に割り当てた。
- `make check` は lint → test → build の順で、バックエンドとフロントエンドの両方を通す。
  web に test スクリプトが無い間、`frontend-test` は `npm test --if-present` で何もしない。
- 見直す条件: 開発者が増えて Windows 環境が入る、または Makefile の依存関係が
  追いにくくなった場合は go-task を再検討する。
