DROP INDEX idx_conversation_participants_identified;
CREATE INDEX idx_conversation_participants_identified
    ON conversation_participants (joined_at)
    WHERE ip_hash IS NOT NULL OR device_fingerprint IS NOT NULL;

COMMENT ON COLUMN conversation_participants.joined_at IS '参加日時。180 日を過ぎると、通報のない会話では ip_hash と device_fingerprint を NULL にする';

ALTER TABLE conversation_participants
    DROP COLUMN age_band,
    DROP COLUMN gender;
