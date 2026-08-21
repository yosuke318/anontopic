CREATE TABLE topics (
    id          SERIAL PRIMARY KEY,
    name        VARCHAR(100) NOT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE conversations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    topic_id        INT NOT NULL REFERENCES topics(id),
    room_type       SMALLINT NOT NULL,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at        TIMESTAMPTZ,
    end_reason      VARCHAR(30),
    is_flagged      BOOLEAN NOT NULL DEFAULT false
);

-- 参加者は実 ID を持たず、一時セッショントークンだけで表す。
CREATE TABLE conversation_participants (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id UUID NOT NULL REFERENCES conversations(id),
    session_token   VARCHAR(64) NOT NULL,
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- created_at のレンジパーティション。保持期間を過ぎた月はパーティションごと削除する。
-- パーティションキーを含まない一意制約は作れないため、主キーは (id, created_at)。
CREATE TABLE messages (
    id              BIGINT GENERATED ALWAYS AS IDENTITY,
    conversation_id UUID NOT NULL REFERENCES conversations(id),
    sender_token    VARCHAR(64) NOT NULL,
    body            TEXT NOT NULL,
    moderation_flag SMALLINT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE TABLE reports (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id UUID NOT NULL REFERENCES conversations(id),
    reporter_token  VARCHAR(64) NOT NULL,
    reason          VARCHAR(100),
    status          VARCHAR(20) NOT NULL DEFAULT 'open',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE banned_identifiers (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    identifier_type VARCHAR(20) NOT NULL,
    identifier      VARCHAR(128) NOT NULL,
    reason          VARCHAR(100),
    banned_until    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_conversations_topic_id ON conversations (topic_id);

-- 削除バッチが除外対象を引くため、フラグ付きの行だけを索引する。
CREATE INDEX idx_conversations_is_flagged ON conversations (is_flagged) WHERE is_flagged;

CREATE INDEX idx_conversation_participants_conversation_id
    ON conversation_participants (conversation_id);

CREATE INDEX idx_messages_conversation_id ON messages (conversation_id, created_at);

CREATE INDEX idx_reports_conversation_id ON reports (conversation_id);
CREATE INDEX idx_reports_status ON reports (status);

-- BAN 判定は接続のたびに引くため、type と identifier の複合で索引する。
CREATE INDEX idx_banned_identifiers_lookup
    ON banned_identifiers (identifier_type, identifier);

-- 当月を挟んで前後の月次パーティションを用意する。継続的な追加と削除は
-- retention モジュールが受け持つ。
DO $$
DECLARE
    first_month date := (date_trunc('month', now()) - interval '6 months')::date;
    month_start date;
BEGIN
    FOR i IN 0..9 LOOP
        month_start := (first_month + (i || ' months')::interval)::date;
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS messages_%s PARTITION OF messages FOR VALUES FROM (%L) TO (%L)',
            to_char(month_start, 'YYYYMM'),
            month_start,
            (month_start + interval '1 month')::date
        );
    END LOOP;
END $$;
