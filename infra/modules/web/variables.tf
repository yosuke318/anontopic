variable "name_prefix" {
  description = "リソース名の接頭辞（例: anontopic-dev）。"
  type        = string
}

variable "vpc_id" {
  description = "ALB のターゲットグループを置く VPC の ID。"
  type        = string
}

variable "private_subnet_ids" {
  description = "タスクを置くプライベートサブネットの ID。"
  type        = list(string)
}

variable "security_group_id" {
  description = "タスクに付けるセキュリティグループの ID。"
  type        = string
}

variable "port" {
  description = "コンテナが待ち受けるポート。network モジュールの web_port と揃える。"
  type        = number
  default     = 3000
}

# --- タスク ---

variable "cluster_arn" {
  description = "サービスを置く ECS クラスターの ARN。"
  type        = string
}

variable "desired_count" {
  description = "常に動かすタスクの数。オートスケーリングは使わず、この数で固定する。"
  type        = number
}

variable "cpu" {
  description = "タスクの vCPU（1024 = 1 vCPU）。Fargate の CPU とメモリの組み合わせから選ぶ。"
  type        = number
  default     = 256
}

variable "memory" {
  description = "タスクのメモリ（MiB）。"
  type        = number
  default     = 512
}

variable "initial_image_tag" {
  description = "Terraform が登録するタスク定義が指すイメージのタグ。サービスを作るときにだけ使い、以降のデプロイはタスク定義の新しいリビジョンで行う。"
  type        = string
  default     = "initial"
}

variable "api_base_url" {
  description = "Next.js のサーバーがトピック一覧を読みに行く API のオリジン（環境変数 API_BASE_URL）。"
  type        = string
}

variable "ecr_force_delete" {
  description = "true のとき、イメージが残っていても ECR リポジトリを削除できる。普段は destroy しておく環境で使う。"
  type        = bool
  default     = false
}

variable "log_retention_days" {
  description = "コンテナのログを CloudWatch Logs に残す日数。"
  type        = number
  default     = 14
}

# --- ALB ---

variable "listener_arn" {
  description = "ルールを足す ALB の HTTPS リスナーの ARN。"
  type        = string
}

variable "listener_rule_priority" {
  description = "CloudFront からの要求をフロントエンドに振り分けるリスナールールの優先度。"
  type        = number
  default     = 100
}

variable "deregistration_delay" {
  description = "タスクを止めるときに、ALB がそのタスクへの既存の接続を残しておく秒数。"
  type        = number
  default     = 30
}

# --- CloudFront とドメイン ---

variable "zone_name" {
  description = "サイトのレコードを作る Route 53 のパブリックホストゾーン名（例: example.com）。"
  type        = string
}

variable "domain_name" {
  description = "サイトのドメイン（例: example.com, dev.example.com）。zone_name の中に置く。"
  type        = string
}

variable "origin_domain_name" {
  description = "CloudFront のオリジンにする ALB のドメイン。ALB の証明書に含まれている名前にする。"
  type        = string
}

variable "price_class" {
  description = "CloudFront の価格クラス。PriceClass_100 は日本のエッジを含まない。"
  type        = string
  default     = "PriceClass_200"
}
