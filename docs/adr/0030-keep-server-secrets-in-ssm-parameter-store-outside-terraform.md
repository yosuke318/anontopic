# ADR-0030: サーバーの秘密は SSM パラメータストアに置き、値は Terraform の外で入れる

- ステータス: Accepted
- 日付: 2026-10-02
- 関連 issue: YOSUKE-136

## 文脈

- サーバーは `DATABASE_URL`・`REDIS_URL`・`SESSION_IP_HASH_SECRET`・`ADMIN_API_TOKEN` を
  環境変数から読む。どれも漏れると DB への接続や管理操作、IP ハッシュの逆算に使われる。
- ECS はタスクの起動時に、SSM パラメータストアか Secrets Manager の値を環境変数として渡せる。
- Terraform の state は S3 に置き、plan 用のロールも読める（ADR-0027）。RDS の issue
  （YOSUKE-137）でも、認証情報を state に平文で残さないことを求めている。
- 月額予算は 4 万円前後で、見積もりにほぼ余裕がない。

## 決定

- 秘密は SSM パラメータストアの SecureString（AWS 管理キー `aws/ssm`）として
  `/anontopic/<env>/api/<環境変数名>` に置き、値は運用者が AWS CLI で入れる。
- Terraform はパラメータを作らず読みもしない。パスから ARN を組み立ててタスク定義に書き、
  タスク実行ロールにそのパスの `ssm:GetParameters` だけを許す。

## 検討した代替案

### 案A: Secrets Manager

1 つあたり月 $0.40 で、4 つ × 2 環境で約 $3.2/月 かかる。今の用途では自動ローテーションを
使わず、パラメータストアの標準パラメータ（無料）で足りる。

### 案B: Terraform でパラメータを作り、値の変更を `ignore_changes` で無視する

値を CLI で書き換えても、リフレッシュのたびに復号した値が state に入る。

### 案C: タスク定義の環境変数に平文で書く

値がタスク定義に残り、コンソールや API と state から読める。

### 案D: Terraform の `random_password` で生成する

生成した値が state に残る。

## 影響

- パラメータが 1 つでも無いと、タスクは起動に失敗する。環境を作るときに README の手順で
  先に入れておく必要がある。
- 値はタスクの起動時に読むため、変えたら新しいデプロイが要る。
- 値の変更はコードレビューを通らず、履歴はパラメータストアのバージョンだけになる。
- 見直す条件: RDS の認証情報を RDS 管理のシークレット（Secrets Manager）で持つと決めたときや、
  ローテーションが必要になったときは、その秘密だけ Secrets Manager に移す。
