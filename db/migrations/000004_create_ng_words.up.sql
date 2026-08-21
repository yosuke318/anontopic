CREATE TABLE ng_words (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    word        VARCHAR(100) NOT NULL,
    category    VARCHAR(30) NOT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ng_words_word_key UNIQUE (word)
);

COMMENT ON TABLE ng_words IS 'NGワード辞書';
COMMENT ON COLUMN ng_words.id IS 'NGワードID';
COMMENT ON COLUMN ng_words.word IS 'NGワード';
COMMENT ON COLUMN ng_words.category IS '分類。meetup / dating / sexual / contact / solicitation';
COMMENT ON COLUMN ng_words.is_active IS '有効フラグ。false の語は判定に使わない';
COMMENT ON COLUMN ng_words.created_at IS '登録日時';
