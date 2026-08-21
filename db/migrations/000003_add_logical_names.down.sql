COMMENT ON TABLE topics IS NULL;
COMMENT ON COLUMN topics.id IS NULL;
COMMENT ON COLUMN topics.name IS NULL;
COMMENT ON COLUMN topics.is_active IS NULL;
COMMENT ON COLUMN topics.created_at IS NULL;

COMMENT ON TABLE conversations IS NULL;
COMMENT ON COLUMN conversations.id IS NULL;
COMMENT ON COLUMN conversations.topic_id IS NULL;
COMMENT ON COLUMN conversations.room_type IS NULL;
COMMENT ON COLUMN conversations.started_at IS NULL;
COMMENT ON COLUMN conversations.ended_at IS NULL;
COMMENT ON COLUMN conversations.end_reason IS NULL;
COMMENT ON COLUMN conversations.is_flagged IS NULL;

COMMENT ON TABLE conversation_participants IS NULL;
COMMENT ON COLUMN conversation_participants.id IS NULL;
COMMENT ON COLUMN conversation_participants.conversation_id IS NULL;
COMMENT ON COLUMN conversation_participants.session_token IS NULL;
COMMENT ON COLUMN conversation_participants.joined_at IS NULL;

COMMENT ON TABLE messages IS NULL;
COMMENT ON COLUMN messages.id IS NULL;
COMMENT ON COLUMN messages.conversation_id IS NULL;
COMMENT ON COLUMN messages.sender_token IS NULL;
COMMENT ON COLUMN messages.body IS NULL;
COMMENT ON COLUMN messages.moderation_flag IS NULL;
COMMENT ON COLUMN messages.created_at IS NULL;

COMMENT ON TABLE reports IS NULL;
COMMENT ON COLUMN reports.id IS NULL;
COMMENT ON COLUMN reports.conversation_id IS NULL;
COMMENT ON COLUMN reports.reporter_token IS NULL;
COMMENT ON COLUMN reports.reason IS NULL;
COMMENT ON COLUMN reports.status IS NULL;
COMMENT ON COLUMN reports.created_at IS NULL;

COMMENT ON TABLE banned_identifiers IS NULL;
COMMENT ON COLUMN banned_identifiers.id IS NULL;
COMMENT ON COLUMN banned_identifiers.identifier_type IS NULL;
COMMENT ON COLUMN banned_identifiers.identifier IS NULL;
COMMENT ON COLUMN banned_identifiers.reason IS NULL;
COMMENT ON COLUMN banned_identifiers.banned_until IS NULL;
COMMENT ON COLUMN banned_identifiers.created_at IS NULL;
