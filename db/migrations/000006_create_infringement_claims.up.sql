-- 権利侵害の申し立ては会話の参加者に限らず誰からでも受け付けるため、会話 ID と
-- 通報者トークンを必須にする reports とは別の表に置く。
CREATE TABLE infringement_claims (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name            VARCHAR(100) NOT NULL,
    email           VARCHAR(254) NOT NULL,
    infringed_right VARCHAR(20) NOT NULL,
    details         TEXT NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'open',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_infringement_claims_status ON infringement_claims (status);

COMMENT ON TABLE infringement_claims IS '権利侵害の申し立て';
COMMENT ON COLUMN infringement_claims.id IS '申し立てID';
COMMENT ON COLUMN infringement_claims.name IS '申立人の氏名または名称';
COMMENT ON COLUMN infringement_claims.email IS '申立人の連絡先メールアドレス';
COMMENT ON COLUMN infringement_claims.infringed_right IS '侵害された権利。privacy / defamation / copyright / other';
COMMENT ON COLUMN infringement_claims.details IS '侵害情報の内容・場所・侵害と考える理由';
COMMENT ON COLUMN infringement_claims.status IS '対応ステータス。open / reviewing / actioned / rejected';
COMMENT ON COLUMN infringement_claims.created_at IS '受付日時';

COMMENT ON COLUMN reports.reason IS '通報理由。dating / sexual / contact / harassment / spam / other';
COMMENT ON COLUMN reports.status IS '対応ステータス。open / reviewing / actioned / rejected';
