locals {
  project = "anontopic"
  env     = "dev"
  region  = "ap-northeast-1"

  # CI では GitHub の変数が未登録だと空文字で渡ってくるため、null と同じに扱う。
  domain_name = var.domain_name == "" ? null : var.domain_name

  app_port = 8080
}
