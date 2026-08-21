-- サービス目的（雑談・趣味・相談）から外れないトピックだけを置く。
-- 属性の指定や出会いを連想させるトピックは作らない。
INSERT INTO topics (name)
SELECT seed.name
FROM (VALUES
    ('雑談'),
    ('趣味'),
    ('相談'),
    ('ゲーム'),
    ('音楽'),
    ('映画・ドラマ'),
    ('勉強・仕事'),
    ('今日あったこと')
) AS seed(name)
WHERE NOT EXISTS (
    SELECT 1 FROM topics t WHERE t.name = seed.name
);
