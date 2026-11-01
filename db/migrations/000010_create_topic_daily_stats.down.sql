DROP INDEX idx_conversations_topic_id_started_at;
CREATE INDEX idx_conversations_topic_id ON conversations (topic_id);

DROP TABLE topic_daily_stats;
