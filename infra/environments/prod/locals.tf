locals {
  project = "anontopic"
  env     = "prod"
  region  = "ap-northeast-1"

  # CI では GitHub の変数やシークレットが未登録だと空文字で渡ってくるため、null と同じに扱う。
  domain_name = var.domain_name == "" ? null : var.domain_name
  alarm_email = var.alarm_email == "" ? null : var.alarm_email

  app_port = 8080
  web_port = 3000
}
