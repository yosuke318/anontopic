variable "name_prefix" {
  description = "リソース名の接頭辞（例: anontopic-prod）。"
  type        = string
}

variable "alarm_email" {
  description = "アラームと予算の通知を受け取るメールアドレス。null のときは SNS トピックだけを作り、購読を作らない。購読は届いたメールのリンクを開いて確認するまで有効にならない。"
  type        = string
  default     = null
}

# --- 予算 ---

variable "budget_name" {
  description = "AWS Budgets の予算の名前。null のときは予算を作らない。予算はアカウント全体の費用を対象にするため、1 つのアカウントで 1 つの環境だけが作る。"
  type        = string
  default     = null
}

variable "monthly_budget_usd" {
  description = "月額の予算（USD）。実際の費用がこの 50% / 80% / 100% を超えたときに通知する。"
  type        = number
  default     = null

  validation {
    condition     = var.monthly_budget_usd == null || try(var.monthly_budget_usd > 0, false)
    error_message = "monthly_budget_usd は正の数で指定する。"
  }
}

# --- 監視対象 ---

variable "ecs_cluster_name" {
  description = "API のサービスがある ECS クラスターの名前。"
  type        = string
}

variable "ecs_service_name" {
  description = "API の ECS サービスの名前。"
  type        = string
}

variable "load_balancer_attached" {
  description = "ECS サービスが ALB のターゲットグループにつながっているか。false のときは ALB のアラームを作らない。"
  type        = bool
}

variable "alb_arn_suffix" {
  description = "ALB の ARN の末尾（CloudWatch のディメンション LoadBalancer の値）。"
  type        = string
}

variable "target_group_arn_suffix" {
  description = "ターゲットグループの ARN の末尾（CloudWatch のディメンション TargetGroup の値）。"
  type        = string
}

variable "db_instance_identifier" {
  description = "RDS のインスタンス識別子。"
  type        = string
}

variable "redis_cluster_id" {
  description = "監視する ElastiCache のノードのクラスター ID（CloudWatch のディメンション CacheClusterId の値）。"
  type        = string
}

variable "nat_instance_id" {
  description = "NAT インスタンスの ID。"
  type        = string
}

# --- しきい値 ---

variable "ecs_cpu_threshold" {
  description = "API のサービスの CPU 使用率（%）の平均がこれを 15 分続けて超えたら通知する。"
  type        = number
  default     = 80
}

variable "ecs_memory_threshold" {
  description = "API のサービスのメモリ使用率（%）の最大がこれを超えたら通知する。"
  type        = number
  default     = 85
}

variable "alb_5xx_threshold" {
  description = "ALB が返した 5xx とターゲットが返した 5xx の 5 分間の合計がこれを超えたら通知する。"
  type        = number
  default     = 10
}

variable "db_cpu_threshold" {
  description = "RDS の CPU 使用率（%）の平均がこれを 15 分続けて超えたら通知する。"
  type        = number
  default     = 80
}

variable "db_connections_threshold" {
  description = "RDS への接続数の最大がこれを超えたら通知する。サーバーの pgxpool は 1 タスクあたり max(4, CPU 数) 本しか張らないため、それを大きく超える接続はアプリ以外か接続の漏れを疑う。"
  type        = number
  default     = 50
}

variable "redis_memory_threshold" {
  description = "Redis のメモリ使用率（%）の最大がこれを超えたら通知する。maxmemory-policy が noeviction のため、使い切ると書き込みが失敗する。"
  type        = number
  default     = 80
}

# --- retention バッチ ---

variable "retention_log_group_name" {
  description = "retention バッチのログを受けるロググループの名前。指定すると、バッチが失敗したとき（ログのメッセージが \"retention failed\"）に通知する。null のときは作らない。"
  type        = string
  default     = null
}
