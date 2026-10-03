variable "name_prefix" {
  description = "リソース名の接頭辞（例: anontopic-dev）。RDS のインスタンス識別子にもなる。"
  type        = string
}

variable "subnet_ids" {
  description = "DB サブネットグループに入れるプライベートサブネットの ID。RDS の制約で 2 つ以上の AZ にまたがる必要がある。"
  type        = list(string)
}

variable "security_group_id" {
  description = "インスタンスに付けるセキュリティグループの ID。"
  type        = string
}

variable "instance_class" {
  description = "インスタンスクラス（例: db.t4g.small）。"
  type        = string
}

variable "database_name" {
  description = "インスタンスの作成時に作るデータベースの名前。"
  type        = string
  default     = "anontopic"
}

variable "master_username" {
  description = "マスターユーザーの名前。運用者だけが使い、アプリは別のロールで接続する。"
  type        = string
  default     = "anontopic_admin"
}

variable "allocated_storage" {
  description = "作成時のストレージの容量（GiB）。gp3 の最小は 20。"
  type        = number
  default     = 20
}

variable "max_allocated_storage" {
  description = "ストレージの自動拡張の上限（GiB）。"
  type        = number
}

variable "backup_retention_days" {
  description = "自動バックアップを残す日数。"
  type        = number

  validation {
    condition     = var.backup_retention_days >= 1 && var.backup_retention_days <= 35
    error_message = "backup_retention_days は 1〜35 で指定する。"
  }
}

variable "log_retention_days" {
  description = "PostgreSQL のログを CloudWatch Logs に残す日数。"
  type        = number
  default     = 14
}
