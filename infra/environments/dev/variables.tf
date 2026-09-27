variable "aws_account_id" {
  description = "Terraform を適用する AWS アカウントの ID。別のアカウントの認証情報で実行したときは plan の時点で止まる。"
  type        = string

  validation {
    condition     = can(regex("^[0-9]{12}$", var.aws_account_id))
    error_message = "aws_account_id は 12 桁の数字で指定する。"
  }
}
