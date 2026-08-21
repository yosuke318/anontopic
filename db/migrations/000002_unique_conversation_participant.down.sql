CREATE INDEX idx_conversation_participants_conversation_id
    ON conversation_participants (conversation_id);

ALTER TABLE conversation_participants
    DROP CONSTRAINT conversation_participants_conversation_id_session_token_key;
