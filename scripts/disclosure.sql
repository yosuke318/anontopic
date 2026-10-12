-- 発信者情報の開示の請求・裁判所の命令・捜査機関からの照会に回答するため、1 つの会話に
-- ついて保有している記録をすべて読み出す。読み取り専用のトランザクションで実行し、
-- 何も書き換えない。手順は docs/takedown-flow.md、判断の理由は
-- docs/adr/0037-keep-only-hashed-sender-identifiers-and-erase-them-after-180-days.md。
--
--   psql "$DATABASE_URL" -v conversation_id=<会話ID> -f scripts/disclosure.sql
--
-- 参加者は、参加の順に振ったルーム内の番号で表す。セッショントークンは
-- 回答に要らないため出さない。

\set ON_ERROR_STOP on

\if :{?conversation_id}
\else
    \warn 'conversation_id を -v conversation_id=<会話ID> で渡してください'
    \quit
\endif

BEGIN READ ONLY;

\echo '== 会話 =='
SELECT c.id, t.name AS topic, c.room_type, c.started_at, c.ended_at, c.end_reason, c.is_flagged
FROM conversations c
JOIN topics t ON t.id = c.topic_id
WHERE c.id = :'conversation_id';

\echo '== 参加者 =='
\echo 'ip_hash と device_fingerprint が空の参加者は、保存期間を過ぎて消えている。'
WITH disclosure_participants AS (
    SELECT
        row_number() OVER (ORDER BY joined_at, id) AS participant,
        session_token, joined_at, ip_hash, device_fingerprint
    FROM conversation_participants
    WHERE conversation_id = :'conversation_id'
)
SELECT participant, joined_at, ip_hash, device_fingerprint
FROM disclosure_participants
ORDER BY participant;

\echo '== メッセージ =='
\echo 'moderation_flag は 0:問題なし、1:NG検知（相手には届いていない）、2:通報あり。'
WITH disclosure_participants AS (
    SELECT
        row_number() OVER (ORDER BY joined_at, id) AS participant,
        session_token, joined_at, ip_hash, device_fingerprint
    FROM conversation_participants
    WHERE conversation_id = :'conversation_id'
)
SELECT p.participant, m.created_at, m.moderation_flag, m.stored_in, m.body
FROM (
    SELECT sender_token, created_at, moderation_flag, body, 'messages' AS stored_in
    FROM messages
    WHERE conversation_id = :'conversation_id'
    UNION ALL
    SELECT sender_token, created_at, moderation_flag, body, 'retained_messages'
    FROM retained_messages
    WHERE conversation_id = :'conversation_id'
) m
LEFT JOIN disclosure_participants p ON p.session_token = m.sender_token
ORDER BY m.created_at;

\echo '== 通報 =='
WITH disclosure_participants AS (
    SELECT
        row_number() OVER (ORDER BY joined_at, id) AS participant,
        session_token, joined_at, ip_hash, device_fingerprint
    FROM conversation_participants
    WHERE conversation_id = :'conversation_id'
)
SELECT r.id, p.participant AS reporter, r.reason, r.status, r.created_at
FROM reports r
LEFT JOIN disclosure_participants p ON p.session_token = r.reporter_token
WHERE r.conversation_id = :'conversation_id'
ORDER BY r.created_at;

\echo '== 制裁 =='
\echo 'この会話をきっかけにした制裁と、参加者の識別子にかかっている制裁。'
WITH disclosure_participants AS (
    SELECT
        row_number() OVER (ORDER BY joined_at, id) AS participant,
        session_token, joined_at, ip_hash, device_fingerprint
    FROM conversation_participants
    WHERE conversation_id = :'conversation_id'
)
SELECT
    b.id, p.participant, b.identifier_type, b.identifier, b.sanction, b.source, b.reason,
    coalesce(b.conversation_id = :'conversation_id', false) AS from_this_conversation,
    b.created_at, b.banned_until, b.lifted_at
FROM banned_identifiers b
LEFT JOIN disclosure_participants p
    ON (b.identifier_type = 'ip_hash' AND b.identifier = p.ip_hash)
    OR (b.identifier_type = 'device_fingerprint' AND b.identifier = p.device_fingerprint)
WHERE b.conversation_id = :'conversation_id' OR p.participant IS NOT NULL
ORDER BY b.created_at, p.participant;

ROLLBACK;
