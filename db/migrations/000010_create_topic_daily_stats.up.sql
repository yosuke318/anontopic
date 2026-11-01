-- 待機列に入った人数を、トピックとルーム種別ごとに日別で残す。数は Redis で数え、
-- 日次バッチがこの表に書き写す。理由は
-- docs/adr/0043-count-the-topic-demand-in-redis-and-keep-only-daily-totals.md。
CREATE TABLE topic_daily_stats (
    stat_date     DATE NOT NULL,
    topic_id      INT NOT NULL REFERENCES topics(id),
    room_type     SMALLINT NOT NULL,
    queued_count  INT NOT NULL DEFAULT 0,
    matched_count INT NOT NULL DEFAULT 0,
    left_count    INT NOT NULL DEFAULT 0,
    total_wait_ms BIGINT NOT NULL DEFAULT 0,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (stat_date, topic_id, room_type)
);

COMMENT ON TABLE topic_daily_stats IS 'トピック別の日次需要';
COMMENT ON COLUMN topic_daily_stats.stat_date IS '集計日（日本時間）';
COMMENT ON COLUMN topic_daily_stats.topic_id IS 'トピックID';
COMMENT ON COLUMN topic_daily_stats.room_type IS '利用者が選んだルーム種別。2:2人ルーム、3:3人ルーム。3人ルームを選び 2 人で成立した人も 3 に数える';
COMMENT ON COLUMN topic_daily_stats.queued_count IS '待機列に入った人数。同じ人が入り直すと、その回数だけ数える';
COMMENT ON COLUMN topic_daily_stats.matched_count IS 'ルームが成立して待機列を出た人数';
COMMENT ON COLUMN topic_daily_stats.left_count IS '成立する前に自分で待機をやめた人数';
COMMENT ON COLUMN topic_daily_stats.total_wait_ms IS '成立した人の待ち時間の合計（ミリ秒）。total_wait_ms / matched_count が平均の待ち時間';
COMMENT ON COLUMN topic_daily_stats.updated_at IS '最後に書き写した日時';

-- トピック別の集計を期間で絞れるよう、開始日時を足した複合インデックスにする。
DROP INDEX idx_conversations_topic_id;
CREATE INDEX idx_conversations_topic_id_started_at ON conversations (topic_id, started_at);
