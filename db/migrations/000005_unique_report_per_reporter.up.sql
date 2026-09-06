-- 1 会話の1 通報者を 1 行とする。会話に付く通報の数を、ボタンが押された回数では
-- なく通報した参加者の人数にするため。
ALTER TABLE reports
    ADD CONSTRAINT reports_conversation_id_reporter_token_key
    UNIQUE (conversation_id, reporter_token);

-- 一意制約が作る索引が conversation_id を先頭に持つため、会話 ID だけで引く
-- 問い合わせもそちらで賄える。
DROP INDEX idx_reports_conversation_id;
