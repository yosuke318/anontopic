-- 日次パーティションは messages のパーティションとしてそのまま使えるため残す。
COMMENT ON COLUMN messages.created_at IS '送信日時。月次パーティションのキー';

DROP TABLE retained_messages;
