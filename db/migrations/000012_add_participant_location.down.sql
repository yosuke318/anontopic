DROP TABLE geo_daily_stats;

DROP INDEX idx_conversation_participants_joined_at;

DROP INDEX idx_conversation_participants_identified;
CREATE INDEX idx_conversation_participants_identified
    ON conversation_participants (joined_at)
    WHERE ip_hash IS NOT NULL OR device_fingerprint IS NOT NULL
        OR gender IS NOT NULL OR age_band IS NOT NULL;

COMMENT ON COLUMN conversation_participants.joined_at IS '参加日時。180 日を過ぎると、通報のない会話では ip_hash・device_fingerprint・gender・age_band を NULL にする';

ALTER TABLE conversation_participants
    DROP COLUMN asn,
    DROP COLUMN prefecture,
    DROP COLUMN country_code;
