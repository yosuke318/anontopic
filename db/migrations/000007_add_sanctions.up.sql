-- 参加者を BAN の識別子で引けるようにする。通報からの制裁と、運営が会話の参加者を
-- 指して行う BAN が、この列から識別子を読む。値が無い参加者は制裁の対象にならない。
ALTER TABLE conversation_participants
    ADD COLUMN ip_hash            VARCHAR(64),
    ADD COLUMN device_fingerprint VARCHAR(64);

-- 警告も同じ表に残し、次の制裁を何段目にするかをこの表の行数で決める。
-- 解除した行は数えない。
ALTER TABLE banned_identifiers
    ADD COLUMN sanction        VARCHAR(20) NOT NULL DEFAULT 'suspension',
    ADD COLUMN source          VARCHAR(20) NOT NULL DEFAULT 'operator',
    ADD COLUMN conversation_id UUID,
    ADD COLUMN lifted_at       TIMESTAMPTZ;

UPDATE banned_identifiers SET sanction = 'permanent' WHERE banned_until IS NULL;

ALTER TABLE banned_identifiers
    ALTER COLUMN sanction DROP DEFAULT,
    ALTER COLUMN source DROP DEFAULT;

COMMENT ON COLUMN conversation_participants.ip_hash IS '参加時のセッションが発行された接続元の鍵付きハッシュ';
COMMENT ON COLUMN conversation_participants.device_fingerprint IS '参加時の端末ID。サーバーが発行して Cookie に置く値';

COMMENT ON COLUMN banned_identifiers.identifier_type IS '識別子種別。ip_hash / device_fingerprint';
COMMENT ON COLUMN banned_identifiers.sanction IS '制裁の段階。warning / suspension / permanent';
COMMENT ON COLUMN banned_identifiers.source IS '制裁のきっかけ。ng_word / report / operator';
COMMENT ON COLUMN banned_identifiers.reason IS '運営が書いた制裁の理由';
COMMENT ON COLUMN banned_identifiers.conversation_id IS '制裁のきっかけになった会話のID';
COMMENT ON COLUMN banned_identifiers.banned_until IS '一時停止の終了日時。warning と permanent は NULL';
COMMENT ON COLUMN banned_identifiers.lifted_at IS '運営が解除した日時。NULL は解除されていない';
