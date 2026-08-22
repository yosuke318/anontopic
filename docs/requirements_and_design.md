# 匿名トピックチャットサービス 要件定義書 / 基本設計書

作成日: 2026-08-20（更新: GeoIPを用いた地域・不正検知設計を追記 2026-08-22）

---

## 第1部 要件定義書

### 1. サービス概要

- **サービス名(仮)**: anontopic
- **コンセプト**: ChatPad型の匿名ランダムチャットサービス。会員登録不要で、トピック（話題）を選んで見知らぬ相手と匿名でテキストチャットできる。
- **目的**: 雑談・趣味・相談などのコミュニケーションの場を提供する。異性交際・出会いの仲介を目的としない。
- **ルーム形式**: 2人ルーム（1対1）と3人ルームの2種類を用意する。3人ルームは第三者の存在による自浄作用と、通報時の証言確保を狙う。

### 2. 非機能要件（規模・予算）

| 項目 | 内容 |
|---|---|
| 想定同時接続数上限 | 1,000人（アプリケーション層でハードキャップを設定） |
| 月額運用コスト目標 | 約4万円前後（定常状態） |
| モデレーション方式 | AI(LLM)は使用しない。NGワード辞書＋通報ベースの一次対応 |
| 会話ログ保持期間 | 原則90日で自動削除。通報された会話のみ例外的に長期保持 |
| インフラ | AWS想定（EC2/ECS, ElastiCache Redis, RDS PostgreSQL） |
| 開発体制 | 個人開発（1名） |

### 3. 法務・コンプライアンス要件

#### 3.1 出会い系サイト規制法（インターネット異性紹介事業）への非該当性確保

以下の4要件を満たすと規制対象になるため、いずれも満たさない設計とする。

1. 面識のない異性との交際希望者の求めに応じない（性別選択・異性交際訴求を機能に持たない）
2. 異性交際に関する情報を掲示板に掲載しない（公開プロフィール・募集一覧を持たない）
3. 閲覧者が投稿者と直接連絡できる導線を作らない（外部連絡先交換を禁止・技術的にブロック）
4. サービス目的を「雑談・趣味・相談」に限定する（規約・広告文言でも一貫させる）

#### 3.2 禁止行為(利用規約に明記)

- 出会い・交際・性的関係・対面を目的とした利用
- 性別・年齢・地域等の属性による相手の指定・検索
- 待ち合わせ、宿泊、飲酒、交際、性交渉の勧誘
- 電話番号、メールアドレス、SNS ID、住所、位置情報等の送受信
- 前各号を助長する表現・隠語・外部誘導

#### 3.3 未成年保護

- 全年齢対象とし、性的表現・出会い目的の利用を技術的・規約的に排除する方針とする（年齢確認は初期リリースでは導入せず、機能制限で担保する）。

#### 3.4 情報流通プラットフォーム対処法への対応

- 通報受付フォームを設置する
- 権利侵害の申し立てに対する削除対応フローを整備する
- 発信者情報（IPアドレス等）を一定期間保存し、裁判所命令等に対応できる体制を持つ

### 4. モデレーション要件

| 層 | 内容 |
|---|---|
| 一次フィルタ | NGワード辞書によるリアルタイム送信ブロック（下ネタ、外部連絡先、待ち合わせ関連語等） |
| 二次対応 | 利用者からの通報受付、蓄積された通報頻度によるアカウント制限 |
| 制裁 | 段階的制裁（警告 → 一時停止 → 恒久停止） |
| AI活用 | 初期リリースでは不使用（コスト理由）。将来導入する場合は、通報された会話のみを対象とした限定運用とする |

### 5. 収益化要件

- Google AdSenseによる広告収益化を想定
- 広告スクリプトは`next/script`の`lazyOnload`戦略等で遅延読み込みし、チャット機能のパフォーマンスへの影響を最小化する
- AdSense審査対応のため、出会い系・性的表現を含まないコンテンツポリシー準拠を徹底する
- 広告収入発生後は開業届・確定申告等の税務対応を行う（運営者個人の対応事項）

---

## 第2部 基本設計書

### 1. アーキテクチャスタイル

**モジュラーモノリス**を採用する。

#### 1.1 選定理由

個人開発（1名）、同時接続1,000人規模、月額予算4万円前後という条件はいずれも「モノリス〜モジュラーモノリス」が適する典型的な条件であり、マイクロサービスは明確にオーバースペックと判断した。

| 判断軸 | マイクロサービスの目安 | 本プロジェクトの状況 |
|---|---|---|
| チーム規模 | 10名以上 | 1名 |
| リリース頻度 | 週1回以上 | 個人開発のため低頻度 |
| インフラコスト | モノリス比 3.75〜6倍 | 予算上限4万円と相反 |
| 運用体制 | 分散トレーシング・監視の専任体制が必要 | 個人運用のため非現実的 |

一方で、単一の巨大なコードベースにベタ書きする「素のモノリス」は将来の拡張性を損なうため、モジュール間に明示的な境界を持たせた**モジュラーモノリス**とする。将来、特定機能（モデレーション等）の負荷が突出した場合に、独立サービスへ切り出す余地を残す。

#### 1.2 モジュール分割方針（Package by Feature）

Goのコードベースは機能単位（Package by Feature）でディレクトリを分割し、各モジュールは自分が担当するテーブルのみを操作する。他モジュールのデータが必要な場合は、必ず当該モジュールが公開する関数・インターフェース経由でアクセスし、DBモデルを直接importしない。

```
/cmd/server/main.go
/internal/
    matching/     -- マッチングキュー・ルーム割当ロジック
    chat/         -- WebSocket接続管理・メッセージ配送
    moderation/   -- NGワードフィルタ・通報時の一次判定・IP/GeoIPベースの不正検知
    report/       -- 通報受付・BAN管理（reports, banned_identifiers）
    topic/        -- トピックのCRUD（topics）
    retention/    -- 90日削除バッチ（messagesパーティション管理）・GeoIPログの保持期間管理
```

#### 1.3 モジュール間の依存ルール

- モジュール間の呼び出しは公開インターフェース経由のみとする（DBテーブルの直接参照禁止）。
- `chat`モジュールは`moderation`モジュールが提供する判定関数を呼び出す形にし、判定ロジックの実装詳細に依存しない。
- `retention`モジュールは`report`モジュールに問い合わせて「フラグ付き会話か」を確認してから削除対象を決定する。

### 2. システム構成概要

```
[ユーザー] --HTTPS/WSS--> [Next.js(フロントエンド)]
                              |
                              | REST API / WebSocket
                              v
                [Go バックエンドサーバー(モジュラーモノリス)]
                  matching / chat / moderation / report / topic / retention
                       |            |
                       v            v
                  [Redis]      [PostgreSQL]
              (マッチングキュー   (topics/conversations/
               ・プレゼンス       messages, 通報ログ,
               ・レート制限      BANリスト,
               ・IP帯域カウンタ)  GeoIP情報)
```

### 3. 技術スタック

| レイヤー | 選定技術 | 選定理由 |
|---|---|---|
| バックエンド | Go (gorilla/websocket) | goroutine + netpollerによりI/Oバウンド・CPUバウンド双方に強く、マルチコアを自動活用できる。開発者の既存スキルとも合致。単一バイナリでモジュラーモノリスを組みやすい |
| フロントエンド | Next.js (TypeScript, App Router) | LP/SEOページはSSR、チャット画面はCSR（Client Component）で使い分け可能。next/scriptによる広告最適化機能あり |
| DB | PostgreSQL | 通報ログ・BANリスト等の構造化データに適する。パーティショニングによる保持期間管理が可能 |
| キャッシュ/キュー | Redis (ElastiCache) | マッチングキュー、プレゼンス管理、Pub/Subによるサーバー間メッセージ配送、レート制限カウンタ、IP帯域別接続カウンタ |
| 地理情報 | MaxMind GeoLite2 (City/ASN、無料) | 都道府県判定・ASNベースのVPN/ホスティング事業者判定に使用。将来的に有料版(GeoIP2 Anonymous IP)へ切替可能 |

### 4. データベース設計

#### 4.1 テーブル一覧

```sql
-- トピック
CREATE TABLE topics (
    id          SERIAL PRIMARY KEY,
    name        VARCHAR(100) NOT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 会話
CREATE TABLE conversations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    topic_id        INT NOT NULL REFERENCES topics(id),
    room_type       SMALLINT NOT NULL,      -- 2:2人ルーム, 3:3人ルーム
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at        TIMESTAMPTZ,
    end_reason      VARCHAR(30),            -- user_left / timeout / reported / system
    is_flagged      BOOLEAN NOT NULL DEFAULT false
);

-- 会話参加者(実IDではなく一時セッショントークンを保持。GeoIP由来情報を付加)
CREATE TABLE conversation_participants (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id UUID NOT NULL REFERENCES conversations(id),
    session_token   VARCHAR(64) NOT NULL,
    ip_hash         VARCHAR(64) NOT NULL,   -- ソルト付きハッシュ化IP(生IPは保存しない)
    ip_subnet       VARCHAR(20),            -- 多重接続検知用(/24等)
    country_code    VARCHAR(2),
    prefecture      VARCHAR(20),
    asn             INT,
    is_suspected_vpn BOOLEAN NOT NULL DEFAULT false,
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- メッセージ(created_atでレンジパーティショニング、90日超過分は削除)
CREATE TABLE messages (
    id              BIGINT GENERATED ALWAYS AS IDENTITY,
    conversation_id UUID NOT NULL REFERENCES conversations(id),
    sender_token    VARCHAR(64) NOT NULL,
    body            TEXT NOT NULL,
    moderation_flag SMALLINT NOT NULL DEFAULT 0, -- 0:問題なし 1:NG検知 2:通報あり
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
) PARTITION BY RANGE (created_at);

-- 通報ログ(長期保持)
CREATE TABLE reports (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id UUID NOT NULL REFERENCES conversations(id),
    reporter_token  VARCHAR(64) NOT NULL,
    reason          VARCHAR(100),
    status          VARCHAR(20) NOT NULL DEFAULT 'open',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- BANリスト
CREATE TABLE banned_identifiers (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    identifier_type VARCHAR(20) NOT NULL,   -- ip_hash / ip_subnet / device_fingerprint
    identifier      VARCHAR(128) NOT NULL,
    reason          VARCHAR(100),
    banned_until    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- GeoIP集計(ダッシュボード表示用の日次サマリ)
CREATE TABLE geo_daily_stats (
    stat_date        DATE NOT NULL,
    country_code     VARCHAR(2),
    prefecture       VARCHAR(20),
    connection_count INT NOT NULL DEFAULT 0,
    report_count     INT NOT NULL DEFAULT 0,
    PRIMARY KEY (stat_date, country_code, prefecture)
);
```

#### 4.2 データ保持ポリシー

- `messages`テーブルは月次パーティション。90日を超えたパーティションは`DROP TABLE`で削除するバッチを深夜帯に実行。
- `is_flagged = true`の会話に紐づく`messages`は、削除バッチの対象から除外（別テーブルへ退避、または保持期間を1年に延長）。
- `reports`, `banned_identifiers`は長期保持（監査・法対応目的）。
- `conversation_participants`のIP/GeoIP関連項目（`ip_hash`, `ip_subnet`, `country_code`, `prefecture`, `asn`, `is_suspected_vpn`）は、会話ログとは別枠で90日〜6ヶ月を目安に自動削除する。総務省ガイドラインで接続認証ログの保存が一般に6ヶ月程度（ネットワーク管理上の必要性が高い場合は1年程度）許容されるとされていることを踏まえた期間設定とする。
- 生のIPアドレスは保存せず、ソルト付きハッシュ化した`ip_hash`と、多重接続検知に必要な範囲の`ip_subnet`のみを保持し、匿名性の原則を維持する。

### 5. インフラ構成

| コンポーネント | 構成 | 月額目安(定常時) |
|---|---|---|
| WebSocketサーバー(Go) | 小型インスタンス2台(冗長化) | 約$40 |
| Redis | 小型ノード1〜2台 | 約$9〜20 |
| PostgreSQL | Multi-AZ、中型インスタンス | 約$70 |
| DBストレージ(90日分) | 約240GB | 約$28 |
| バックアップ | 定常分 | 約$23 |
| データ転送(egress) | 1,000同時接続想定 | 約$109 |
| GeoIPデータベース | MaxMind GeoLite2(無料)を使用 | $0 |
| **合計目安** | | **約$280(≒4.2万円)/月** |

### 6. コスト上限の歯止め設計

1. **アプリケーション層**: Redisカウンタで同時接続数を管理し、上限（1,000）到達時は新規接続を待機列に入れるか拒否する。
2. **インフラ層**: オートスケーリングは使わず、固定台数構成とする。
3. **課金アラート**: AWS Budgetsで月額予算アラート（50%/80%/100%）を設定。可能であれば予算超過時に新規マッチング受付を自動停止するLambda連携を検討する。

### 7. モデレーション実装方針

- NGワード辞書によるメッセージ送信前フィルタ（正規表現/部分一致）
- URL・電話番号・メールアドレス・SNS IDらしき文字列パターンの検知とブロック
- 通報ボタン・ブロックボタンをチャットUIに常設
- AI判定は初期リリースでは導入しない（コスト理由）。将来導入する場合は、通報された会話のみを対象とした限定運用とする。

### 8. フロントエンド設計方針

- **LP/紹介ページ**: Next.js App RouterのServer Componentを使用しSSR/SSGで生成。SEO・広告収益化を意識。
- **チャットルーム画面**: Client Component（`'use client'`）でCSRとし、WebSocket接続・リアルタイム描画に特化。ハイドレーションコストを避ける。
- **広告表示**: `next/script`の`strategy="lazyOnload"`（または`worker`）でAdSenseスクリプトを遅延読み込みし、チャット機能のパフォーマンスへの影響を抑制。

### 9. GeoIPを用いた地域・不正検知設計

#### 9.1 目的

マッチング条件（性別・年齢・地域による相手指定）には一切使用しない。あくまで以下3つのモデレーション・分析用途に限定する。

1. 同一IP帯からの多重接続検知
2. 特定地域からの荒らし集中の検知（ダッシュボード把握、広告出稿の参考）
3. 海外VPN/プロキシ経由の疑いがある異常アクセスの検知

#### 9.2 使用するデータソース

| 用途 | データソース | コスト |
|---|---|---|
| 都道府県判定 | MaxMind GeoLite2 City（無料） | $0 |
| VPN/プロキシ疑いの判定(初期) | GeoLite2 ASN（無料）+ 既知のホスティング/VPN事業者ASNリストとの照合 | $0 |
| VPN/プロキシ判定(精度向上時) | GeoIP2 Anonymous IP Database（`is_anonymous_vpn`等） | 有料(将来検討) |

初期リリースでは無料のASNベース判定から開始し、誤検知・見逃しの実測値を見ながら有料版への切替を検討する。

#### 9.3 実装方式

- **多重接続検知**: 接続確立時に`ip_subnet`（例: /24）をキーにしたカウンタをRedisでインクリメントし、一定時間内（例: 5分）に閾値（例: 10接続）を超えたら`moderation`モジュールがレート制限を発動する。
- **地域集中検知**: `reports`登録時に`conversation_participants.prefecture`を突き合わせ、`geo_daily_stats`を日次バッチで更新する。通報率が閾値を超えた都道府県・ASNを検知したら管理者に通知する。
- **VPN/プロキシ疑い検知**: 接続確立時にASNを引き、既知のホスティング/VPN事業者リストと照合して`is_suspected_vpn`を立てる。即時ブロックはせず、レート制限強化・NGワード判定閾値の引き下げなど段階的な扱いにする（正規の海外在住ユーザーの誤遮断を避けるため）。

#### 9.4 プライバシー配慮

- 生IPアドレスは保存せず、ソルト付きハッシュ化した`ip_hash`と、多重接続検知に必要な範囲の`ip_subnet`のみを保持する。
- 法的照会等でどうしても生IPが必要になる場合に備え、アクセス制限をかけた最小限の別テーブルで短期間のみ保持する運用を将来検討する（未実装）。
- 保持期間は会話ログ(90日)とは別枠で管理し、6ヶ月程度を目安に`retention`モジュールで自動削除する。

### 10. 今後の検討事項（未確定）

- 通報ワークフローの詳細（誰がレビューするか、対応SLA）
- NGワード辞書の初期セットと運用中のチューニング方法
- セッショントークンの発行・失効ロジックの詳細
- 3人ルームのマッチング待機時間の許容上限
- 決済・課金機能を将来的に追加するか否か
- モジュラーモノリスからの将来的な切り出し候補の見極め（負荷計測後に判断）
- ホスティング/VPN事業者ASNリストの具体的な取得元・更新方法
- 有料GeoIP2 Anonymous IP Databaseへの切替タイミングの判断基準
