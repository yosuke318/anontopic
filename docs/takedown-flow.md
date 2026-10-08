# 通報と権利侵害の申し立ての対応手順

利用者からの通報と、第三者からの権利侵害の申し立てを受けたときに、運営者がとる手順をまとめる。
管理 API の使い方は [README の「通報」と「BAN と段階的制裁」](../README.md#ban-と段階的制裁)、
判断の理由は ADR-0021・ADR-0022・ADR-0023・ADR-0025・ADR-0037 にある。

この文書は運営者向けの手順で、利用者に約束している内容は利用規約（`/terms`）と
プライバシーポリシー（`/privacy`）に書いている。両者がずれたら、公開している側に手順を合わせる。

## 前提

- 会話は参加者どうしにしか見えず、会話が終わった後は参加者も読み返せない。
  サービス上で第三者に公開され続けている投稿は無い。
- 進行中の会話は、通報された時点で終わる（ADR-0021）。
- メッセージは 90 日で消える。通報のあった会話のメッセージだけは `retained_messages` に移して残す
  （ADR-0026）。
- 接続元は IP アドレスそのものではなく、鍵付きのハッシュ値（`ip_hash`）でしか持っていない。
  ポート番号と User-Agent も持っていない。
- 参加者の `ip_hash` と `device_fingerprint` は、参加から 180 日で retention バッチが消す。
  通報のあった会話の参加者の分だけは残す（ADR-0037）。

## 利用者からの通報

1. 未対応の通報を一覧する。

   ```bash
   curl -H "Authorization: Bearer $TOKEN" "$API/api/admin/reports?status=open"
   ```

2. 確認を始めたら `reviewing` にし、会話ログを読む。参加者はルーム内の番号で出る。

   ```bash
   curl -X PATCH -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
     -d '{"status":"reviewing"}' "$API/api/admin/reports/<id>"
   curl -H "Authorization: Bearer $TOKEN" "$API/api/admin/reports/<id>"
   ```

3. 利用規約の禁止行為（第 4 条）に当たるかを判断する。

   | 判断 | 措置 | ステータス |
   | --- | --- | --- |
   | 当たる | 該当する参加者の端末 ID に制裁をかける。同じ回線から繰り返していると読み取れるときだけ、IP ハッシュにもかける | `actioned` |
   | 当たらない | 何もしない。通報が重なったことによる自動の制裁がかかっていたら解除する | `rejected` |

   ```bash
   curl -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
     -d '{"conversation_id":"<会話ID>","participant":2,"identifier_type":"device_fingerprint","sanction":"suspension","duration_hours":72,"reason":"<禁止行為>"}' \
     "$API/api/admin/bans"
   curl -X PATCH -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
     -d '{"status":"actioned"}' "$API/api/admin/reports/<id>"
   ```

IP ハッシュへの制裁は、同じ回線（学校・モバイル回線など）の利用者を全員止める。端末 ID への
制裁を先に使う。

## 権利侵害の申し立て

1. 未対応の申し立てを一覧する。氏名（名称）・メールアドレス・権利の種類・内容が返る。

   ```bash
   curl -H "Authorization: Bearer $TOKEN" "$API/api/admin/claims?status=open"
   ```

2. 確認を始めたら `reviewing` にする。

   ```bash
   curl -X PATCH -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
     -d '{"status":"reviewing"}' "$API/api/admin/claims/<id>"
   ```

3. 該当する会話を探す。申し立ては会話を特定せず、管理 API には会話を探す手段が無い。
   [infra/README.md の「アプリのロールを作る」](../infra/README.md#アプリのロールを作る)と同じ
   ポートフォワードでデータベースに入り、申し立てに書かれた時期と語句で `messages` と
   `retained_messages` を検索する。送信から 90 日を過ぎていて通報も無かった会話は、
   メッセージがすでに消えている。

4. 申し立てた方にメールで返事をする。

   | 状況 | 返事の内容 | 措置 | ステータス |
   | --- | --- | --- | --- |
   | 会話が見つかり、権利侵害に当たる | 確認した結果と、とった措置 | 書き込んだ参加者に制裁をかける | `actioned` |
   | 会話が見つからない、またはすでに消えている | 該当する記録が無いこと | なし | `rejected` |
   | 権利侵害に当たらない | 判断の結果 | なし | `rejected` |

   会話はすでに終わっていて誰からも見えないため、公開を止める措置（送信防止措置）は要らない。
   返事では、内容が第三者に公開されていないことも伝える。

5. 申し立ての記録は消さない。法令にもとづく照会に備えて残す。

## 発信者情報の開示の請求・裁判所の命令・捜査機関からの照会

回答できるのは、会話の参加と送信の日時、メッセージ、`ip_hash`、`device_fingerprint`、制裁の記録だけで、
IP アドレスそのものは回答できない。それでも、正当な請求には保有している範囲で回答する。

### 1. 受け付けて記録を作る

請求ごとに対応記録を作り、次を書き込んでいく。請求者や対象の情報を含むため、公開リポジトリには
置かず、運営者の手元に残す。対応を終えた後も消さない。

| 項目 | 内容 |
| --- | --- |
| 受付日・経路 | 郵送 / メール / 窓口 など |
| 請求者 | 裁判所・警察署・弁護士会などの名称と担当者 |
| 根拠 | 開示命令、提供命令、捜索差押許可状、捜査関係事項照会、弁護士会照会 など |
| 対象 | 請求に書かれた日時・語句・IP アドレス・会話 ID |
| 抽出 | 抽出した日時、対象の会話 ID |
| 回答 | 回答日、回答した内容、回答しなかった内容とその理由 |

### 2. 本人性と正当性を確かめる

- 根拠が分からない請求、個人がメールで「開示してほしい」と求めるだけの請求には応じない。
  裁判所の手続（発信者情報開示命令）があることだけを伝える。
- 裁判所の命令や令状は、書面で届いたことを確かめる。
- 捜査機関からの照会や弁護士会照会は、書面の照会書で受ける。電話やメールだけで届いたときは、
  記載された番号ではなく、警察署・弁護士会の代表番号にかけ直して、照会が本物かを確かめる。
- 本サービスの会話は参加者どうしにしか届かないため、情報流通プラットフォーム対処法の開示の
  対象になるかが確かでない。開示命令が届いたとき、または回答の要否に迷うときは、回答する前に
  弁護士に相談する。

### 3. 対象の会話を特定する

[infra/README.md の「アプリのロールを作る」](../infra/README.md#アプリのロールを作る)と同じ
ポートフォワードでデータベースに入る（`dev` を `prod` に読み替える）。

- **会話 ID が示されているとき**はそのまま 4 へ進む。
- **日時と語句が示されているとき**は、`messages` と `retained_messages` を検索する。送信から 90 日を
  過ぎていて通報も無かった会話は、メッセージがすでに消えている。

  ```sql
  SELECT conversation_id, created_at, body FROM messages
  WHERE created_at BETWEEN '<開始>' AND '<終了>' AND body LIKE '%<語句>%'
  UNION ALL
  SELECT conversation_id, created_at, body FROM retained_messages
  WHERE created_at BETWEEN '<開始>' AND '<終了>' AND body LIKE '%<語句>%';
  ```

- **IP アドレスが示されているとき**は、サーバーと同じ鍵でハッシュ値にして参加記録を引く。参加から
  180 日を過ぎた、通報の無い会話の参加者は、ハッシュ値が消えていて見つからない。

  ```bash
  secret="$(aws ssm get-parameter --with-decryption \
    --name /anontopic/prod/api/SESSION_IP_HASH_SECRET --query Parameter.Value --output text)"
  printf '%s' '<IP アドレス>' | openssl dgst -sha256 -hmac "$secret" | awk '{print $NF}'
  ```

  ```sql
  SELECT conversation_id, joined_at FROM conversation_participants
  WHERE ip_hash = '<ハッシュ値>' ORDER BY joined_at;
  ```

### 4. 記録を抽出する

[scripts/disclosure.sql](../scripts/disclosure.sql) に会話 ID を渡すと、会話・参加者・メッセージ・
通報・制裁を読み出す。読み取り専用のトランザクションで動き、何も書き換えない。

```bash
psql "host=localhost port=15432 dbname=anontopic user=<ロール> sslmode=require" \
  -v conversation_id=<会話ID> -f scripts/disclosure.sql -o disclosure-<会話ID>.txt
```

手元で試すときは、`make up` で起こしたデータベースにシードの会話を指定する。

```bash
docker compose exec -T postgres psql -U anontopic -d anontopic \
  -v conversation_id=22222222-2222-2222-2222-222222222222 -f - < scripts/disclosure.sql
```

### 5. 回答する

- 請求の範囲に含まれる項目だけを回答する。会話の他の参加者の記録は、請求の対象でなければ含めない。
- `ip_hash` と `device_fingerprint` を回答するときは、「IP アドレスを鍵付きのハッシュ値にしたもの」
  「本サービスがブラウザごとに発行した乱数」であることを添える。
- IP アドレス、ポート番号、User-Agent、氏名・住所・電話番号などは「保有していない」と回答する。
  期間を過ぎて消えた情報は「保存期間を過ぎたため保有していない」と回答する。
- 回答したファイルは、回答を終えたら手元から消す。対応記録には、何を回答したかだけを残す。

### 6. 抽出の跡

ポートフォワードを開いたことは CloudTrail の `StartSession` に、データベースのパスワードや
`SESSION_IP_HASH_SECRET` を読んだことは `GetSecretValue` / `GetParameter` に、誰がいつ行ったかとともに残る。イベント履歴は 90 日分しか残らないため、抽出した日時と
会話 ID は 1 の対応記録に必ず書く。

### 鍵を変えない

`SESSION_IP_HASH_SECRET` を変えると、それまでの `ip_hash` は示された IP アドレスと照合できなくなり、
`ip_hash` にかけた制裁も効かなくなる。鍵が漏れたとき以外は変えない。
