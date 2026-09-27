-- 列を落とすと警告と解除済みの行が無期限の BAN として読まれるため、先に消す。
DELETE FROM banned_identifiers WHERE sanction = 'warning' OR lifted_at IS NOT NULL;

ALTER TABLE banned_identifiers
    DROP COLUMN lifted_at,
    DROP COLUMN conversation_id,
    DROP COLUMN source,
    DROP COLUMN sanction;

COMMENT ON COLUMN banned_identifiers.reason IS '制限理由';
COMMENT ON COLUMN banned_identifiers.banned_until IS '制限解除日時。NULL は無期限';

ALTER TABLE conversation_participants
    DROP COLUMN device_fingerprint,
    DROP COLUMN ip_hash;
