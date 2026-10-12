-- retention が参加から 180 日を過ぎた参加者の識別子を NULL にする。識別子の残る行だけを
-- 参加日時で引けるようにする。理由は
-- docs/adr/0037-keep-only-hashed-sender-identifiers-and-erase-them-after-180-days.md。
CREATE INDEX idx_conversation_participants_identified
    ON conversation_participants (joined_at)
    WHERE ip_hash IS NOT NULL OR device_fingerprint IS NOT NULL;

COMMENT ON COLUMN conversation_participants.joined_at IS '参加日時。180 日を過ぎると、通報のない会話では ip_hash と device_fingerprint を NULL にする';
