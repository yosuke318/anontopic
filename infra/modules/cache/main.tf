# Redis は 1 ノードで動かし、レプリカと自動フェイルオーバーは持たない（ADR-0031）。
# ノードを足せば作り直さずに Multi-AZ にできるよう、レプリケーショングループで作る。

resource "aws_elasticache_subnet_group" "this" {
  name       = var.name_prefix
  subnet_ids = var.subnet_ids
}

resource "aws_elasticache_parameter_group" "this" {
  name   = "${var.name_prefix}-redis7"
  family = "redis7"

  # マッチングキューや接続のリースが黙って追い出されると、マッチングや同時接続数の数え方が
  # 崩れる。メモリが尽きたらキーを追い出さず、書き込みを失敗させる。
  parameter {
    name  = "maxmemory-policy"
    value = "noeviction"
  }

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_elasticache_replication_group" "this" {
  replication_group_id = var.name_prefix
  description          = "Sessions, matching queue, presence and rate limits"

  engine               = "redis"
  engine_version       = "7.1"
  node_type            = var.node_type
  num_cache_clusters   = 1
  port                 = 6379
  parameter_group_name = aws_elasticache_parameter_group.this.name

  subnet_group_name  = aws_elasticache_subnet_group.this.name
  security_group_ids = [var.security_group_id]

  automatic_failover_enabled = false
  multi_az_enabled           = false

  # 接続は TLS に限る。AUTH トークンは state に残るため使わず、到達はセキュリティグループで絞る。
  at_rest_encryption_enabled = true
  transit_encryption_enabled = true

  snapshot_retention_limit = 0

  # 時刻は UTC。月曜 04:30 JST から。
  maintenance_window         = "sun:19:30-sun:20:30"
  auto_minor_version_upgrade = true
}
