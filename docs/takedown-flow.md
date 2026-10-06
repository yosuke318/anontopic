# 通報と権利侵害の申し立ての対応手順

利用者からの通報と、第三者からの権利侵害の申し立てを受けたときに、運営者がとる手順をまとめる。
管理 API の使い方は [README の「通報」と「BAN と段階的制裁」](../README.md#ban-と段階的制裁)、
判断の理由は ADR-0021・ADR-0022・ADR-0023・ADR-0025 にある。

この文書は運営者向けの手順で、利用者に約束している内容は利用規約（`/terms`）と
プライバシーポリシー（`/privacy`）に書いている。両者がずれたら、公開している側に手順を合わせる。

## 前提

- 会話は参加者どうしにしか見えず、会話が終わった後は参加者も読み返せない。
  サービス上で第三者に公開され続けている投稿は無い。
- 進行中の会話は、通報された時点で終わる（ADR-0021）。
- メッセージは 90 日で消える。通報のあった会話のメッセージだけは `retained_messages` に移して残す
  （ADR-0026）。
- 接続元は IP アドレスそのものではなく、鍵付きのハッシュ値（`ip_hash`）でしか持っていない。

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

1. 請求の根拠（情報流通プラットフォーム対処法、裁判所の命令、刑事訴訟法にもとづく照会など）と、
   対象の会話・日時を確かめる。根拠が分からない請求には応じない。
2. 対象の会話の参加者について、保有している情報を集める。
   - 会話の参加記録（`conversation_participants`）: `ip_hash`、`device_fingerprint`
   - メッセージ（`messages` / `retained_messages`）: 本文と送信日時
   - 制裁の記録（`banned_identifiers`）
3. IP アドレスそのものは持っていない。請求する側が IP アドレスを示した場合は、同じ鍵
   （`SESSION_IP_HASH_SECRET`）でハッシュ値にして照合できる。保有していない情報は
   「保有していない」と回答する。
4. 判断に迷うときは、回答する前に弁護士に相談する。
