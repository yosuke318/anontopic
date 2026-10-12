variable "aws_account_id" {
  description = "Terraform を適用する AWS アカウントの ID。別のアカウントの認証情報で実行したときは plan の時点で止まる。"
  type        = string

  validation {
    condition     = can(regex("^[0-9]{12}$", var.aws_account_id))
    error_message = "aws_account_id は 12 桁の数字で指定する。"
  }
}

variable "domain_name" {
  description = "サービスのドメイン（Route 53 のパブリックホストゾーン名）。null か空文字のときは ACM の証明書・DNS レコード・HTTPS リスナーを作らない。"
  type        = string
  default     = null
}

variable "load_test" {
  description = "負荷試験の構成にする。API のタスク数と RDS のクラスを prod に揃え、1 台のクライアントから全員分をつなげるよう IP アドレスごとの上限を外す。"
  type        = bool
  default     = false
}
