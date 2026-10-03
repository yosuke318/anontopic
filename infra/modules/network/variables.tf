variable "name_prefix" {
  description = "リソース名の接頭辞（例: anontopic-dev）。"
  type        = string
}

variable "vpc_cidr" {
  description = "VPC の CIDR。/16 を前提に、サブネットを /24 で切り出す。"
  type        = string

  validation {
    condition     = can(cidrnetmask(var.vpc_cidr)) && endswith(var.vpc_cidr, "/16")
    error_message = "vpc_cidr は /16 の CIDR で指定する。"
  }
}

variable "availability_zones" {
  description = "サブネットを置くアベイラビリティゾーン。ALB と RDS のサブネットグループが 2 つ以上の AZ を求めるため、2 つ以上を指定する。"
  type        = list(string)

  validation {
    condition     = length(var.availability_zones) >= 2
    error_message = "availability_zones は 2 つ以上指定する。"
  }
}

variable "app_port" {
  description = "アプリのコンテナが待ち受けるポート。ALB からの転送先になる。"
  type        = number
  default     = 8080
}

variable "nat_instance_type" {
  description = "NAT インスタンスのインスタンスタイプ。AMI が arm64 のため Graviton のタイプを指定する。"
  type        = string
  default     = "t4g.nano"
}

variable "interface_endpoint_services" {
  description = "プライベートサブネットに作るインターフェイス型 VPC エンドポイントのサービス名（例: ecr.api, logs）。空なら作らず、NAT インスタンスを経由する。"
  type        = list(string)
  default     = []
}
