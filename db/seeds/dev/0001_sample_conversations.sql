-- 通報一覧の表示と削除バッチの動作確認に使うサンプル。
--
-- 何度実行しても同じ状態になるよう、会話は ID を固定して ON CONFLICT DO NOTHING で入れ、
-- 会話にぶら下がる行は「その会話が投入済みか」で判定する。既に投入された会話に
-- 行を足したい場合は make downd で作り直す。

-- 通常どおり終了した 2 人ルーム。
INSERT INTO conversations (id, topic_id, room_type, started_at, ended_at, end_reason, is_flagged)
SELECT
    '11111111-1111-1111-1111-111111111111'::uuid,
    t.id, 2,
    now() - interval '2 hours',
    now() - interval '110 minutes',
    'user_left',
    false
FROM topics t
WHERE t.name = '雑談'
ON CONFLICT (id) DO NOTHING;

-- 通報されてフラグが立った 3 人ルーム。削除バッチの除外対象になる。
INSERT INTO conversations (id, topic_id, room_type, started_at, ended_at, end_reason, is_flagged)
SELECT
    '22222222-2222-2222-2222-222222222222'::uuid,
    t.id, 3,
    now() - interval '1 day',
    now() - interval '1 day' + interval '8 minutes',
    'reported',
    true
FROM topics t
WHERE t.name = '相談'
ON CONFLICT (id) DO NOTHING;

-- 保持期間（90 日）を超えた会話。削除バッチの対象になる。
INSERT INTO conversations (id, topic_id, room_type, started_at, ended_at, end_reason, is_flagged)
SELECT
    '33333333-3333-3333-3333-333333333333'::uuid,
    t.id, 2,
    now() - interval '100 days',
    now() - interval '100 days' + interval '5 minutes',
    'timeout',
    false
FROM topics t
WHERE t.name = 'ゲーム'
ON CONFLICT (id) DO NOTHING;

INSERT INTO conversation_participants (conversation_id, session_token, joined_at)
SELECT seed.conversation_id::uuid, seed.session_token, seed.joined_at
FROM (VALUES
    ('11111111-1111-1111-1111-111111111111', 'devseed-conv1-participant-a', now() - interval '2 hours'),
    ('11111111-1111-1111-1111-111111111111', 'devseed-conv1-participant-b', now() - interval '2 hours'),
    ('22222222-2222-2222-2222-222222222222', 'devseed-conv2-participant-a', now() - interval '1 day'),
    ('22222222-2222-2222-2222-222222222222', 'devseed-conv2-participant-b', now() - interval '1 day'),
    ('22222222-2222-2222-2222-222222222222', 'devseed-conv2-participant-c', now() - interval '1 day'),
    ('33333333-3333-3333-3333-333333333333', 'devseed-conv3-participant-a', now() - interval '100 days'),
    ('33333333-3333-3333-3333-333333333333', 'devseed-conv3-participant-b', now() - interval '100 days')
) AS seed(conversation_id, session_token, joined_at)
WHERE NOT EXISTS (
    SELECT 1
    FROM conversation_participants p
    WHERE p.conversation_id = seed.conversation_id::uuid
      AND p.session_token = seed.session_token
);

INSERT INTO messages (conversation_id, sender_token, body, moderation_flag, created_at)
SELECT seed.conversation_id::uuid, seed.sender_token, seed.body, seed.moderation_flag, seed.created_at
FROM (VALUES
    ('11111111-1111-1111-1111-111111111111', 'devseed-conv1-participant-a', 'こんばんは', 0, now() - interval '2 hours'),
    ('11111111-1111-1111-1111-111111111111', 'devseed-conv1-participant-b', 'こんばんは、今日は寒いですね', 0, now() - interval '119 minutes'),
    ('11111111-1111-1111-1111-111111111111', 'devseed-conv1-participant-a', 'ほんとに。もう暖房つけました', 0, now() - interval '118 minutes'),
    ('22222222-2222-2222-2222-222222222222', 'devseed-conv2-participant-a', '相談したいことがあって', 0, now() - interval '1 day'),
    ('22222222-2222-2222-2222-222222222222', 'devseed-conv2-participant-b', 'どうぞ', 0, now() - interval '1 day' + interval '1 minute'),
    ('22222222-2222-2222-2222-222222222222', 'devseed-conv2-participant-b', 'うん', 0, now() - interval '1 day' + interval '2 minutes'),
    ('22222222-2222-2222-2222-222222222222', 'devseed-conv2-participant-b', 'うん', 0, now() - interval '1 day' + interval '3 minutes'),
    ('22222222-2222-2222-2222-222222222222', 'devseed-conv2-participant-c', '規約違反として通報された発言', 2, now() - interval '1 day' + interval '5 minutes'),
    ('33333333-3333-3333-3333-333333333333', 'devseed-conv3-participant-a', '最近やってるゲームある？', 0, now() - interval '100 days'),
    ('33333333-3333-3333-3333-333333333333', 'devseed-conv3-participant-b', 'ずっと同じのやってる', 0, now() - interval '100 days' + interval '2 minutes')
) AS seed(conversation_id, sender_token, body, moderation_flag, created_at)
-- 同じ送信者が同じ本文を繰り返すことは普通に起きるため、本文ではメッセージを
-- 区別できない。会話単位で投入済みかどうかを見る。
WHERE NOT EXISTS (
    SELECT 1
    FROM messages m
    WHERE m.conversation_id = seed.conversation_id::uuid
);

INSERT INTO reports (conversation_id, reporter_token, reason, status, created_at)
SELECT
    '22222222-2222-2222-2222-222222222222'::uuid,
    'devseed-conv2-participant-a',
    '規約違反の疑い',
    'open',
    now() - interval '1 day' + interval '6 minutes'
WHERE NOT EXISTS (
    SELECT 1
    FROM reports r
    WHERE r.conversation_id = '22222222-2222-2222-2222-222222222222'::uuid
);
