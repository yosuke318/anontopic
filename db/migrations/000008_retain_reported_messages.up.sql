-- 保持期間を過ぎたパーティションを削除する前に、通報された会話のメッセージを移す先。
-- 理由は docs/adr/0026-drop-daily-message-partitions-and-move-reported-messages-aside.md。
-- id は messages で採番された値をそのまま持つ。
CREATE TABLE retained_messages (
    id              BIGINT PRIMARY KEY,
    conversation_id UUID NOT NULL REFERENCES conversations(id),
    sender_token    VARCHAR(64) NOT NULL,
    body            TEXT NOT NULL,
    moderation_flag SMALLINT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,
    retained_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_retained_messages_conversation_id ON retained_messages (conversation_id, created_at);

COMMENT ON TABLE retained_messages IS '保持延長メッセージ。通報された会話のメッセージを、パーティションの削除前に移したもの';
COMMENT ON COLUMN retained_messages.id IS 'メッセージID。messages での値';
COMMENT ON COLUMN retained_messages.conversation_id IS '会話ID';
COMMENT ON COLUMN retained_messages.sender_token IS '送信者トークン';
COMMENT ON COLUMN retained_messages.body IS '本文';
COMMENT ON COLUMN retained_messages.moderation_flag IS 'モデレーション判定。0:問題なし、1:NG検知、2:通報あり';
COMMENT ON COLUMN retained_messages.created_at IS '送信日時';
COMMENT ON COLUMN retained_messages.retained_at IS 'messages から移した日時';

COMMENT ON COLUMN messages.created_at IS '送信日時。日次パーティションのキー（UTC の 0 時区切り）';

-- messages を日次パーティションで持つ。行の無い、今以降を受け持つパーティションを
-- 削除し、その範囲と今日から 14 日先までを日次パーティションで埋める。行のある
-- パーティションは期限が来て retention が削除するまでそのまま使う。
DO $$
DECLARE
    part         record;
    has_rows     boolean;
    walk_from    timestamptz;
    horizon      timestamptz;
    day_start    timestamptz;
BEGIN
    -- 日の区切りとパーティション境界のリテラルを UTC で揃える。
    PERFORM set_config('TimeZone', 'UTC', true);

    walk_from := date_trunc('day', now());
    horizon := walk_from + interval '14 days';

    FOR part IN
        SELECT
            c.oid::regclass AS rel,
            substring(pg_get_expr(c.relpartbound, c.oid) FROM $re$FROM \('([^']+)'\)$re$)::timestamptz AS lower_bound,
            substring(pg_get_expr(c.relpartbound, c.oid) FROM $re$TO \('([^']+)'\)$re$)::timestamptz AS upper_bound
        FROM pg_inherits i
        JOIN pg_class c ON c.oid = i.inhrelid
        WHERE i.inhparent = 'messages'::regclass
    LOOP
        CONTINUE WHEN part.lower_bound IS NULL OR part.upper_bound <= now();

        EXECUTE format('SELECT EXISTS (SELECT 1 FROM %s)', part.rel) INTO has_rows;
        CONTINUE WHEN has_rows;

        EXECUTE format('DROP TABLE %s', part.rel);
        walk_from := least(walk_from, part.lower_bound);
    END LOOP;

    day_start := walk_from;
    WHILE day_start < horizon LOOP
        IF NOT EXISTS (
            SELECT 1
            FROM (
                SELECT
                    substring(pg_get_expr(c.relpartbound, c.oid) FROM $re$FROM \('([^']+)'\)$re$)::timestamptz AS lower_bound,
                    substring(pg_get_expr(c.relpartbound, c.oid) FROM $re$TO \('([^']+)'\)$re$)::timestamptz AS upper_bound
                FROM pg_inherits i
                JOIN pg_class c ON c.oid = i.inhrelid
                WHERE i.inhparent = 'messages'::regclass
            ) b
            WHERE b.lower_bound < day_start + interval '1 day'
              AND b.upper_bound > day_start
        ) THEN
            EXECUTE format(
                'CREATE TABLE %I PARTITION OF messages FOR VALUES FROM (%L) TO (%L)',
                'messages_' || to_char(day_start, 'YYYYMMDD'),
                day_start,
                day_start + interval '1 day'
            );
        END IF;

        day_start := day_start + interval '1 day';
    END LOOP;
END $$;
