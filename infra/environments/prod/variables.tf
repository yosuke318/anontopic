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

variable "alarm_email" {
  description = "アラームと予算の通知を受け取るメールアドレス。null か空文字のときは通知先を登録しない。"
  type        = string
  default     = null
}
