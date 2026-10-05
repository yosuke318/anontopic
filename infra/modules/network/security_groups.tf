# 通信を許すのは インターネット → ALB → アプリ → RDS / ElastiCache の向き、
# インターネット → ALB → フロントエンドの向きと、運用者が NAT インスタンスを経由して
# RDS に入る向きだけにする。
# RDS と ElastiCache のグループは外向きのルールを持たない。

resource "aws_security_group" "alb" {
  name        = "${var.name_prefix}-alb"
  description = "ALB: HTTPS from the internet"
  vpc_id      = aws_vpc.this.id

  tags = {
    Name = "${var.name_prefix}-alb"
  }
}

resource "aws_security_group" "app" {
  name        = "${var.name_prefix}-app"
  description = "App: traffic from the ALB only"
  vpc_id      = aws_vpc.this.id

  tags = {
    Name = "${var.name_prefix}-app"
  }
}

resource "aws_security_group" "web" {
  name        = "${var.name_prefix}-web"
  description = "Web: traffic from the ALB only"
  vpc_id      = aws_vpc.this.id

  tags = {
    Name = "${var.name_prefix}-web"
  }
}

resource "aws_security_group" "database" {
  name        = "${var.name_prefix}-database"
  description = "RDS: PostgreSQL from the app only"
  vpc_id      = aws_vpc.this.id

  tags = {
    Name = "${var.name_prefix}-database"
  }
}

resource "aws_security_group" "cache" {
  name        = "${var.name_prefix}-cache"
  description = "ElastiCache: Redis from the app only"
  vpc_id      = aws_vpc.this.id

  tags = {
    Name = "${var.name_prefix}-cache"
  }
}

# --- ALB ---

resource "aws_vpc_security_group_ingress_rule" "alb_https" {
  security_group_id = aws_security_group.alb.id
  description       = "HTTPS from the internet"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_vpc_security_group_egress_rule" "alb_to_app" {
  security_group_id            = aws_security_group.alb.id
  description                  = "Forward to the app"
  ip_protocol                  = "tcp"
  from_port                    = var.app_port
  to_port                      = var.app_port
  referenced_security_group_id = aws_security_group.app.id
}

resource "aws_vpc_security_group_egress_rule" "alb_to_web" {
  security_group_id            = aws_security_group.alb.id
  description                  = "Forward to the web frontend"
  ip_protocol                  = "tcp"
  from_port                    = var.web_port
  to_port                      = var.web_port
  referenced_security_group_id = aws_security_group.web.id
}

# --- アプリ ---

resource "aws_vpc_security_group_ingress_rule" "app_from_alb" {
  security_group_id            = aws_security_group.app.id
  description                  = "From the ALB"
  ip_protocol                  = "tcp"
  from_port                    = var.app_port
  to_port                      = var.app_port
  referenced_security_group_id = aws_security_group.alb.id
}

resource "aws_vpc_security_group_egress_rule" "app_to_database" {
  security_group_id            = aws_security_group.app.id
  description                  = "PostgreSQL"
  ip_protocol                  = "tcp"
  from_port                    = 5432
  to_port                      = 5432
  referenced_security_group_id = aws_security_group.database.id
}

resource "aws_vpc_security_group_egress_rule" "app_to_cache" {
  security_group_id            = aws_security_group.app.id
  description                  = "Redis"
  ip_protocol                  = "tcp"
  from_port                    = 6379
  to_port                      = 6379
  referenced_security_group_id = aws_security_group.cache.id
}

# ECR・CloudWatch Logs・Grafana Cloud など、外へ出る通信は HTTPS に限る。
resource "aws_vpc_security_group_egress_rule" "app_https" {
  security_group_id = aws_security_group.app.id
  description       = "HTTPS to AWS APIs and external services"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  cidr_ipv4         = "0.0.0.0/0"
}

# --- フロントエンド ---

resource "aws_vpc_security_group_ingress_rule" "web_from_alb" {
  security_group_id            = aws_security_group.web.id
  description                  = "From the ALB"
  ip_protocol                  = "tcp"
  from_port                    = var.web_port
  to_port                      = var.web_port
  referenced_security_group_id = aws_security_group.alb.id
}

# ECR・CloudWatch Logs と、サーバー側の描画で呼ぶ API の公開ドメインへの通信。
resource "aws_vpc_security_group_egress_rule" "web_https" {
  security_group_id = aws_security_group.web.id
  description       = "HTTPS to AWS APIs and the public API endpoint"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  cidr_ipv4         = "0.0.0.0/0"
}

# --- RDS / ElastiCache ---

resource "aws_vpc_security_group_ingress_rule" "database_from_app" {
  security_group_id            = aws_security_group.database.id
  description                  = "PostgreSQL from the app"
  ip_protocol                  = "tcp"
  from_port                    = 5432
  to_port                      = 5432
  referenced_security_group_id = aws_security_group.app.id
}

# 運用者は NAT インスタンスへの Session Manager のポートフォワードで RDS に入り、
# ロールの作成やマイグレーションを行う（ADR-0032）。
resource "aws_vpc_security_group_ingress_rule" "database_from_nat" {
  security_group_id            = aws_security_group.database.id
  description                  = "PostgreSQL from operators through the NAT instance"
  ip_protocol                  = "tcp"
  from_port                    = 5432
  to_port                      = 5432
  referenced_security_group_id = aws_security_group.nat.id
}

resource "aws_vpc_security_group_ingress_rule" "cache_from_app" {
  security_group_id            = aws_security_group.cache.id
  description                  = "Redis from the app"
  ip_protocol                  = "tcp"
  from_port                    = 6379
  to_port                      = 6379
  referenced_security_group_id = aws_security_group.app.id
}
