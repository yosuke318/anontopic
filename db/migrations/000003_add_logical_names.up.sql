COMMENT ON TABLE topics IS 'トピック';
COMMENT ON COLUMN topics.id IS 'トピックID';
COMMENT ON COLUMN topics.name IS 'トピック名';
COMMENT ON COLUMN topics.is_active IS '有効フラグ。false のトピックは一覧に出さない';
COMMENT ON COLUMN topics.created_at IS '作成日時';

COMMENT ON TABLE conversations IS '会話';
COMMENT ON COLUMN conversations.id IS '会話ID';
COMMENT ON COLUMN conversations.topic_id IS 'トピックID';
COMMENT ON COLUMN conversations.room_type IS 'ルーム種別。2:2人ルーム、3:3人ルーム';
COMMENT ON COLUMN conversations.started_at IS '開始日時';
COMMENT ON COLUMN conversations.ended_at IS '終了日時。進行中は NULL';
COMMENT ON COLUMN conversations.end_reason IS '終了理由。user_left / timeout / reported / system';
COMMENT ON COLUMN conversations.is_flagged IS '通報フラグ。true の会話は保持期間を過ぎても削除しない';

COMMENT ON TABLE conversation_participants IS '会話参加者';
COMMENT ON COLUMN conversation_participants.id IS '会話参加者ID';
COMMENT ON COLUMN conversation_participants.conversation_id IS '会話ID';
COMMENT ON COLUMN conversation_participants.session_token IS 'セッショントークン。参加者を会話の中で識別する一時的な値';
COMMENT ON COLUMN conversation_participants.joined_at IS '参加日時';

COMMENT ON TABLE messages IS 'メッセージ';
COMMENT ON COLUMN messages.id IS 'メッセージID';
COMMENT ON COLUMN messages.conversation_id IS '会話ID';
COMMENT ON COLUMN messages.sender_token IS '送信者トークン';
COMMENT ON COLUMN messages.body IS '本文';
COMMENT ON COLUMN messages.moderation_flag IS 'モデレーション判定。0:問題なし、1:NG検知、2:通報あり';
COMMENT ON COLUMN messages.created_at IS '送信日時。月次パーティションのキー';

COMMENT ON TABLE reports IS '通報';
COMMENT ON COLUMN reports.id IS '通報ID';
COMMENT ON COLUMN reports.conversation_id IS '会話ID';
COMMENT ON COLUMN reports.reporter_token IS '通報者トークン';
COMMENT ON COLUMN reports.reason IS '通報理由';
COMMENT ON COLUMN reports.status IS '対応ステータス。既定は open';
COMMENT ON COLUMN reports.created_at IS '通報日時';

COMMENT ON TABLE banned_identifiers IS 'BANリスト';
COMMENT ON COLUMN banned_identifiers.id IS 'BANリストID';
COMMENT ON COLUMN banned_identifiers.identifier_type IS '識別子種別。ip_hash / device_fingerprint';
COMMENT ON COLUMN banned_identifiers.identifier IS '識別子';
COMMENT ON COLUMN banned_identifiers.reason IS '制限理由';
COMMENT ON COLUMN banned_identifiers.banned_until IS '制限解除日時。NULL は無期限';
COMMENT ON COLUMN banned_identifiers.created_at IS '登録日時';
