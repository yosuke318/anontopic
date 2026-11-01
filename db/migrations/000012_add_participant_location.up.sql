-- 接続元の IP アドレスから引いた国・都道府県・ASN を、会話の参加者に残す。統計と
-- モデレーションにだけ使い、マッチングには使わない。IP アドレスそのものは残さない。理由は
-- docs/adr/0045-locate-the-prefecture-of-a-session-with-geolite-downloaded-by-the-server.md。
ALTER TABLE conversation_participants
    ADD COLUMN country_code VARCHAR(2),
    ADD COLUMN prefecture   VARCHAR(6),
    ADD COLUMN asn          BIGINT;

COMMENT ON COLUMN conversation_participants.country_code IS '接続元の国（ISO 3166-1 alpha-2）。NULL は不明。参加から 180 日で NULL にする';
COMMENT ON COLUMN conversation_participants.prefecture IS '接続元の都道府県（ISO 3166-2、例: JP-13）。日本の接続元だけに入る。NULL は不明。参加から 180 日で NULL にする';
COMMENT ON COLUMN conversation_participants.asn IS '接続元のネットワークの AS 番号。NULL は不明。参加から 180 日で NULL にする';
COMMENT ON COLUMN conversation_participants.joined_at IS '参加日時。180 日を過ぎると、通報のない会話では識別子（ip_hash・device_fingerprint）、属性（gender・age_band）、接続元（country_code・prefecture・asn）を NULL にする';

-- retention が 180 日を過ぎた参加者の識別子・属性・接続元を消すため、どれかが残る行を参加日時で引けるようにする。
DROP INDEX idx_conversation_participants_identified;
CREATE INDEX idx_conversation_participants_identified
    ON conversation_participants (joined_at)
    WHERE ip_hash IS NOT NULL OR device_fingerprint IS NOT NULL
        OR gender IS NOT NULL OR age_band IS NOT NULL
        OR country_code IS NOT NULL OR prefecture IS NOT NULL OR asn IS NOT NULL;

-- 日次バッチが参加日時で 1 日分の参加者を数えるため。
CREATE INDEX idx_conversation_participants_joined_at ON conversation_participants (joined_at);

-- 国と都道府県ごとの日次の集計。個人に結びつかない数だけを持ち、長く残す。
CREATE TABLE geo_daily_stats (
    stat_date        DATE NOT NULL,
    country_code     VARCHAR(2) NOT NULL DEFAULT '',
    prefecture       VARCHAR(6) NOT NULL DEFAULT '',
    connection_count INT NOT NULL DEFAULT 0,
    report_count     INT NOT NULL DEFAULT 0,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (stat_date, country_code, prefecture)
);

COMMENT ON TABLE geo_daily_stats IS '国・都道府県別の日次集計';
COMMENT ON COLUMN geo_daily_stats.stat_date IS '集計日（日本時間）';
COMMENT ON COLUMN geo_daily_stats.country_code IS '国（ISO 3166-1 alpha-2）。空文字は不明';
COMMENT ON COLUMN geo_daily_stats.prefecture IS '都道府県（ISO 3166-2）。空文字は不明か日本以外';
COMMENT ON COLUMN geo_daily_stats.connection_count IS 'その日に会話に参加した人数';
COMMENT ON COLUMN geo_daily_stats.report_count IS 'その日にあった通報について、通報した人を除く会話の参加者を数えた数';
COMMENT ON COLUMN geo_daily_stats.updated_at IS '最後に集計した日時';
