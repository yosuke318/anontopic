-- messages のパーティションは親テーブルと一緒に削除される。
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS banned_identifiers;
DROP TABLE IF EXISTS reports;
DROP TABLE IF EXISTS conversation_participants;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS topics;
