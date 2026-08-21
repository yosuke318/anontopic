-- NG ワード辞書の初期セット。禁止行為（利用規約）に対応する語を分類ごとに置く。
--
-- ここに並ぶのは出発点であって完成品ではない。誤検知は必ず出るので、運用しながら
-- is_active を落としたり語を足したりして調整する。表記揺れの正規化と判定ロジックは
-- moderation モジュールが受け持つ。
INSERT INTO ng_words (word, category)
SELECT seed.word, seed.category
FROM (VALUES
    -- 待ち合わせ・対面
    ('会いたい', 'meetup'),
    ('会おう', 'meetup'),
    ('会える', 'meetup'),
    ('待ち合わせ', 'meetup'),
    ('直接会', 'meetup'),
    ('オフ会', 'meetup'),
    ('泊まり', 'meetup'),
    ('泊めて', 'meetup'),
    ('ホテル', 'meetup'),
    ('家に来', 'meetup'),
    ('飲みに行こう', 'meetup'),
    ('今から行く', 'meetup'),

    -- 交際・出会い
    ('付き合っ', 'dating'),
    ('付き合お', 'dating'),
    ('恋人', 'dating'),
    ('彼氏募集', 'dating'),
    ('彼女募集', 'dating'),
    ('交際', 'dating'),
    ('デートし', 'dating'),
    ('出会い目的', 'dating'),
    ('マッチング希望', 'dating'),

    -- 性的な語
    ('エッチ', 'sexual'),
    ('えっち', 'sexual'),
    ('セックス', 'sexual'),
    ('ヤリモク', 'sexual'),
    ('下ネタ', 'sexual'),
    ('童貞', 'sexual'),
    ('処女', 'sexual'),
    ('裸', 'sexual'),
    ('巨乳', 'sexual'),

    -- 外部連絡先の交換
    ('連絡先', 'contact'),
    ('LINE交換', 'contact'),
    ('ライン交換', 'contact'),
    ('ID交換', 'contact'),
    ('アイディー交換', 'contact'),
    ('インスタ', 'contact'),
    ('カカオ', 'contact'),
    ('ディスコ', 'contact'),
    ('電話番号', 'contact'),
    ('メアド', 'contact'),
    ('メールアドレス', 'contact'),

    -- 勧誘・スパム
    ('稼げ', 'solicitation'),
    ('副業', 'solicitation'),
    ('儲か', 'solicitation'),
    ('高収入', 'solicitation'),
    ('無料登録', 'solicitation'),
    ('招待コード', 'solicitation'),
    ('当選しました', 'solicitation')
) AS seed(word, category)
WHERE NOT EXISTS (
    SELECT 1 FROM ng_words n WHERE n.word = seed.word
);
