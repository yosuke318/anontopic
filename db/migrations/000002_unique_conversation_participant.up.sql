-- 1 会話につき 1 参加者 1 行とする。再接続しても行は増やさない。
ALTER TABLE conversation_participants
    ADD CONSTRAINT conversation_participants_conversation_id_session_token_key
    UNIQUE (conversation_id, session_token);

-- 一意制約が作る索引が conversation_id を先頭に持つため、会話 ID だけで引く
-- 問い合わせもそちらで賄える。
DROP INDEX idx_conversation_participants_conversation_id;
