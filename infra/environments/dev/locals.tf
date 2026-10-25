locals {
  project = "anontopic"
  env     = "dev"
  region  = "ap-northeast-1"

  # CI では GitHub の変数が未登録だと空文字で渡ってくるため、null と同じに扱う。
  domain_name = var.domain_name == "" ? null : var.domain_name

  # 負荷試験のクライアントは 1 つの IP アドレスから全員分をつなぐ。全体の上限（既定 1,000）が
  # 先に効くよう、IP アドレスごとの接続数とマッチングの要求数をそれより大きくする。
  load_test_environment = var.load_test ? {
    CAPACITY_MAX_CONNECTIONS_PER_IP = "10000"
    CAPACITY_MATCH_BURST            = "10000"
  } : {}

  app_port = 8080
  web_port = 3000
}
