CREATE INDEX idx_reports_conversation_id ON reports (conversation_id);

ALTER TABLE reports
    DROP CONSTRAINT reports_conversation_id_reporter_token_key;
