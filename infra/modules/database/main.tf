# PostgreSQL は Single-AZ のインスタンス 1 台で動かす（ADR-0031）。

resource "aws_db_subnet_group" "this" {
  name       = var.name_prefix
  subnet_ids = var.subnet_ids

  tags = {
    Name = var.name_prefix
  }
}

# RDS が書き出すログのロググループを先に作り、保持期間を決める。
resource "aws_cloudwatch_log_group" "postgresql" {
  name              = "/aws/rds/instance/${var.name_prefix}/postgresql"
  retention_in_days = var.log_retention_days
}

# max_connections はインスタンスのメモリから決まる既定値のままにする。サーバーの pgxpool は
# 1 タスクあたり max(4, CPU 数) 本しか張らない。
resource "aws_db_parameter_group" "this" {
  name   = "${var.name_prefix}-postgres18"
  family = "postgres18"

  parameter {
    name  = "rds.force_ssl"
    value = "1"
  }

  parameter {
    name  = "log_min_duration_statement"
    value = "1000"
  }

  parameter {
    name  = "log_lock_waits"
    value = "1"
  }

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_db_instance" "this" {
  identifier     = var.name_prefix
  engine         = "postgres"
  engine_version = "18"
  instance_class = var.instance_class

  db_name  = var.database_name
  username = var.master_username

  # パスワードは RDS が Secrets Manager に作って持つ。state には入らない（ADR-0032）。
  manage_master_user_password = true

  allocated_storage     = var.allocated_storage
  max_allocated_storage = var.max_allocated_storage
  storage_type          = "gp3"
  storage_encrypted     = true

  multi_az               = false
  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [var.security_group_id]
  publicly_accessible    = false
  parameter_group_name   = aws_db_parameter_group.this.name

  # 時刻は UTC。バックアップは 03:00 JST、メンテナンスは月曜 04:00 JST から。
  backup_retention_period = var.backup_retention_days
  backup_window           = "18:00-18:30"
  maintenance_window      = "sun:19:00-sun:19:30"
  copy_tags_to_snapshot   = true

  auto_minor_version_upgrade      = true
  enabled_cloudwatch_logs_exports = ["postgresql"]

  # 消えても作り直せばよい前提で、削除の保護と最後のスナップショットは付けない。
  deletion_protection      = false
  skip_final_snapshot      = true
  delete_automated_backups = true

  depends_on = [aws_cloudwatch_log_group.postgresql]
}
