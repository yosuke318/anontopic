variable "name_prefix" {
  description = "リソース名の接頭辞（例: anontopic-dev）。"
  type        = string
}

variable "vpc_id" {
  description = "ALB のターゲットグループを置く VPC の ID。"
  type        = string
}

variable "public_subnet_ids" {
  description = "ALB を置くパブリックサブネットの ID。2 つ以上の AZ にまたがる必要がある。"
  type        = list(string)
}

variable "private_subnet_ids" {
  description = "タスクを置くプライベートサブネットの ID。"
  type        = list(string)
}

variable "alb_security_group_id" {
  description = "ALB に付けるセキュリティグループの ID。"
  type        = string
}

variable "app_security_group_id" {
  description = "タスクに付けるセキュリティグループの ID。"
  type        = string
}

variable "app_port" {
  description = "コンテナが待ち受けるポート。network モジュールの app_port と揃える。"
  type        = number
  default     = 8080
}

# --- タスク ---

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

variable "environment" {
  description = "コンテナに渡す環境変数のうち、秘密でないもの。APP_ADDR と APP_TRUST_FORWARDED_FOR はモジュールが設定する。"
  type        = map(string)
  default     = {}
}

variable "ssm_parameter_path" {
  description = "秘密の環境変数を置く SSM パラメータストアのパス（例: /anontopic/dev/api）。パラメータは Terraform の外で作る。"
  type        = string

  validation {
    condition     = startswith(var.ssm_parameter_path, "/") && !endswith(var.ssm_parameter_path, "/")
    error_message = "ssm_parameter_path は / で始め、末尾に / を付けない。"
  }
}

variable "secret_names" {
  description = "ssm_parameter_path の下から読む環境変数の名前。パラメータ名と環境変数名は同じにする。"
  type        = list(string)
  default     = []
}

variable "log_retention_days" {
  description = "コンテナのログを CloudWatch Logs に残す日数。"
  type        = number
  default     = 14
}

# --- ALB ---

variable "idle_timeout" {
  description = "ALB が無通信の接続を切るまでの秒数。サーバーが WebSocket に送る ping の間隔（chat.DefaultPingInterval、30 秒）より長くする。"
  type        = number
  default     = 120
}

variable "deregistration_delay" {
  description = "タスクを止めるときに、ALB がそのタスクへの既存の接続を残しておく秒数。過ぎると残っている WebSocket は切られ、ブラウザは別のタスクにつなぎ直す。"
  type        = number
  default     = 300
}

# --- ドメイン ---

variable "zone_name" {
  description = "API のレコードを作る Route 53 のパブリックホストゾーン名（例: example.com）。null のときは ACM の証明書・DNS レコード・HTTPS リスナーを作らず、ALB に届く経路が無い状態になる。"
  type        = string
  default     = null
}

variable "api_record_name" {
  description = "zone_name の下に作る API のレコード名（例: api, api.dev）。"
  type        = string
  default     = "api"
}
