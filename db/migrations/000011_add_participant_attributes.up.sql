-- 利用者が任意で答えた性別と年代を、会話の参加者に残す。統計とモデレーションにだけ使い、
-- マッチングにも相手への表示にも使わない。NULL は答えなかったことを表す。理由は
-- docs/adr/0044-record-the-gender-and-age-band-a-user-chose-to-give-without-matching-on-them.md。
ALTER TABLE conversation_participants
    ADD COLUMN gender VARCHAR(10)
        CONSTRAINT conversation_participants_gender_check
        CHECK (gender IN ('female', 'male', 'other')),
    ADD COLUMN age_band VARCHAR(10)
        CONSTRAINT conversation_participants_age_band_check
        CHECK (age_band IN ('under_18', '18_19', '20s', '30s', '40s', '50s', '60_plus'));

COMMENT ON COLUMN conversation_participants.gender IS '性別（自己申告）。female / male / other。NULL は未回答。参加から 180 日で NULL にする';
COMMENT ON COLUMN conversation_participants.age_band IS '年代（自己申告）。under_18 / 18_19 / 20s / 30s / 40s / 50s / 60_plus。NULL は未回答。参加から 180 日で NULL にする';
COMMENT ON COLUMN conversation_participants.joined_at IS '参加日時。180 日を過ぎると、通報のない会話では ip_hash・device_fingerprint・gender・age_band を NULL にする';

-- retention が 180 日を過ぎた参加者の識別子と属性を消すため、どちらかが残る行を参加日時で引けるようにする。
DROP INDEX idx_conversation_participants_identified;
CREATE INDEX idx_conversation_participants_identified
    ON conversation_participants (joined_at)
    WHERE ip_hash IS NOT NULL OR device_fingerprint IS NOT NULL
        OR gender IS NOT NULL OR age_band IS NOT NULL;
