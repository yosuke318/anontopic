# ADR-0039: アプリ・RDS・ElastiCache・NAT を最初の AZ に寄せて置く

- ステータス: Accepted
- 日付: 2026-10-12
- 関連 issue: YOSUKE-144

## 文脈

- RDS と ElastiCache は Single-AZ の 1 台（ADR-0031）、NAT インスタンスは 1 台で
  パブリックサブネットの先頭に置く（ADR-0028）。これらが止まるとサービス全体が止まるため、
  アプリのタスクを複数の AZ に散らしても、AZ の障害に対する可用性は上がらない。
- このサービスは個人の趣味で運営し、利用者との約束（SLA）は無い。月額予算は 4 万円前後
  （ADR-0031）。
- ALB は 2 つ以上の AZ のサブネットを、RDS の DB サブネットグループも 2 つ以上の AZ の
  サブネットを必要とする。VPC を 1 つの AZ だけにはできない。
- 今は RDS と ElastiCache の AZ を指定しておらず、AWS が決める。アプリのタスクは
  2 つのプライベートサブネットに散らばる。同じ VPC の中でも、AZ をまたぐ通信は
  1 GB あたり双方向で合計 0.02 USD かかる。チャットのメッセージはすべて Redis を通り、
  セッション・キュー・リースも Redis に置くため（ADR-0005・0008・0013）、アプリと Redis の
  間の通信が多い。
- RDS の AZ は作成後に変えると作り直しになる。dev も prod もまだ作っていない。

## 決定

- サブネットは 2 つの AZ に作ったままにする。
- network モジュールが `availability_zones` の先頭を `primary_availability_zone` として出力し、
  その AZ のプライベートサブネットを `primary_private_subnet_id` として出力する。
- API とフロントエンドの ECS タスクは `primary_private_subnet_id` だけに置く。
  RDS と ElastiCache は `primary_availability_zone` に置く。NAT インスタンスは
  先頭のパブリックサブネットに置くため、これも同じ AZ になる。
- ALB は 2 つの AZ のパブリックサブネットに置く。ALB からタスクへの通信は AZ をまたいでも
  課金されない。

## 検討した代替案

### 案A: AZ を指定せず AWS に任せる

AZ の組み合わせが作成のたびに変わり、アプリと Redis の間の通信が AZ をまたぐかどうかを
予測できない。やめた。

### 案B: ECS を 2 つの AZ に散らしたままにする

可用性は RDS・Redis・NAT のどれかが止まれば失われるため、上がらない。AZ をまたぐ通信の
費用だけが増える。

### 案C: 2 つ目の AZ のサブネットを作らない

ALB と DB サブネットグループが作れない。

### 案D: Multi-AZ にして AZ をまたぐ費用を払う

ADR-0031 で退けた。

## 影響

- 最初の AZ（`ap-northeast-1a`）が止まるか、Fargate の容量が足りなくなると、タスクは
  別の AZ で起動できない。復旧は ADR-0031 の手順（別の AZ へのリストア）に加えて、
  タスクを置くサブネットを変える。
- 2 つ目の AZ のプライベートサブネットは、DB と ElastiCache のサブネットグループを
  満たすためだけに残る。タスクは置かない。
- AZ をまたぐ通信の費用がなくなる。実際の転送量は負荷試験（YOSUKE-144）で測る。
- 見直す条件: RDS か ElastiCache を Multi-AZ にするとき、タスクのサブネットも
  `private_subnet_ids` の全部に戻す。
