# インフラのアラーム。どれも AWS が無料で送る標準のメトリクスを見る。
# 状態が戻ったときも通知し、メールだけで復旧まで追えるようにする。

locals {
  alarm_actions = [aws_sns_topic.alerts.arn]
}

# --- ECS（API のサービス） ---

resource "aws_cloudwatch_metric_alarm" "ecs_cpu" {
  alarm_name        = "${var.name_prefix}-api-cpu"
  alarm_description = "API のサービスの CPU 使用率が 15 分続けて ${var.ecs_cpu_threshold}% を超えている。"

  namespace   = "AWS/ECS"
  metric_name = "CPUUtilization"
  dimensions = {
    ClusterName = var.ecs_cluster_name
    ServiceName = var.ecs_service_name
  }
  statistic           = "Average"
  period              = 300
  evaluation_periods  = 3
  comparison_operator = "GreaterThanThreshold"
  threshold           = var.ecs_cpu_threshold
  treat_missing_data  = "missing"

  alarm_actions = local.alarm_actions
  ok_actions    = local.alarm_actions
}

resource "aws_cloudwatch_metric_alarm" "ecs_memory" {
  alarm_name        = "${var.name_prefix}-api-memory"
  alarm_description = "API のサービスのメモリ使用率が ${var.ecs_memory_threshold}% を超えた。使い切るとタスクが強制終了され、その上の WebSocket がすべて切れる。"

  namespace   = "AWS/ECS"
  metric_name = "MemoryUtilization"
  dimensions = {
    ClusterName = var.ecs_cluster_name
    ServiceName = var.ecs_service_name
  }
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 1
  comparison_operator = "GreaterThanThreshold"
  threshold           = var.ecs_memory_threshold
  treat_missing_data  = "missing"

  alarm_actions = local.alarm_actions
  ok_actions    = local.alarm_actions
}

# --- ALB ---

resource "aws_cloudwatch_metric_alarm" "alb_unhealthy" {
  count = var.load_balancer_attached ? 1 : 0

  alarm_name        = "${var.name_prefix}-api-unhealthy-targets"
  alarm_description = "ALB のヘルスチェック（/healthz）に失敗しているタスクが 3 分続けてある。"

  namespace   = "AWS/ApplicationELB"
  metric_name = "UnHealthyHostCount"
  dimensions = {
    LoadBalancer = var.alb_arn_suffix
    TargetGroup  = var.target_group_arn_suffix
  }
  statistic           = "Maximum"
  period              = 60
  evaluation_periods  = 3
  comparison_operator = "GreaterThanThreshold"
  threshold           = 0
  treat_missing_data  = "notBreaching"

  alarm_actions = local.alarm_actions
  ok_actions    = local.alarm_actions
}

# 正常なタスクが 1 つも無いときは ALB 自身が 503 を返すため、ELB の 5xx も合わせて数える。
# ELB の 5xx はターゲットグループごとに分かれないため、フロントエンドへの要求の分もここに入る。
resource "aws_cloudwatch_metric_alarm" "alb_5xx" {
  count = var.load_balancer_attached ? 1 : 0

  alarm_name        = "${var.name_prefix}-api-5xx"
  alarm_description = "ALB と API のタスクが返した 5xx が 5 分間で ${var.alb_5xx_threshold} 件を超えた。"

  evaluation_periods  = 1
  comparison_operator = "GreaterThanThreshold"
  threshold           = var.alb_5xx_threshold
  treat_missing_data  = "notBreaching"

  metric_query {
    id          = "total"
    expression  = "FILL(elb, 0) + FILL(target, 0)"
    label       = "5xx responses"
    return_data = true
  }

  metric_query {
    id = "elb"

    metric {
      namespace   = "AWS/ApplicationELB"
      metric_name = "HTTPCode_ELB_5XX_Count"
      dimensions  = { LoadBalancer = var.alb_arn_suffix }
      stat        = "Sum"
      period      = 300
    }
  }

  metric_query {
    id = "target"

    metric {
      namespace   = "AWS/ApplicationELB"
      metric_name = "HTTPCode_Target_5XX_Count"
      dimensions = {
        LoadBalancer = var.alb_arn_suffix
        TargetGroup  = var.target_group_arn_suffix
      }
      stat   = "Sum"
      period = 300
    }
  }

  alarm_actions = local.alarm_actions
  ok_actions    = local.alarm_actions
}

# --- フロントエンド ---

resource "aws_cloudwatch_metric_alarm" "web_cpu" {
  count = var.web_attached ? 1 : 0

  alarm_name        = "${var.name_prefix}-web-cpu"
  alarm_description = "フロントエンドのサービスの CPU 使用率が 15 分続けて ${var.ecs_cpu_threshold}% を超えている。"

  namespace   = "AWS/ECS"
  metric_name = "CPUUtilization"
  dimensions = {
    ClusterName = var.ecs_cluster_name
    ServiceName = var.web_service_name
  }
  statistic           = "Average"
  period              = 300
  evaluation_periods  = 3
  comparison_operator = "GreaterThanThreshold"
  threshold           = var.ecs_cpu_threshold
  treat_missing_data  = "missing"

  alarm_actions = local.alarm_actions
  ok_actions    = local.alarm_actions
}

resource "aws_cloudwatch_metric_alarm" "web_memory" {
  count = var.web_attached ? 1 : 0

  alarm_name        = "${var.name_prefix}-web-memory"
  alarm_description = "フロントエンドのサービスのメモリ使用率が ${var.ecs_memory_threshold}% を超えた。使い切るとタスクが強制終了され、置き換わるまでサイトが応答しない。"

  namespace   = "AWS/ECS"
  metric_name = "MemoryUtilization"
  dimensions = {
    ClusterName = var.ecs_cluster_name
    ServiceName = var.web_service_name
  }
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 1
  comparison_operator = "GreaterThanThreshold"
  threshold           = var.ecs_memory_threshold
  treat_missing_data  = "missing"

  alarm_actions = local.alarm_actions
  ok_actions    = local.alarm_actions
}

# タスクは 1 つだけなので、異常なタスクの数ではなく正常なタスクが残っているかを見る。
# タスクが消えてターゲットが 0 になったときも、正常なタスクの数は 0 になる。
resource "aws_cloudwatch_metric_alarm" "web_no_healthy_targets" {
  count = var.web_attached ? 1 : 0

  alarm_name        = "${var.name_prefix}-web-no-healthy-targets"
  alarm_description = "ALB のヘルスチェック（/）に通るフロントエンドのタスクが 3 分続けて 1 つも無い。サイトが応答していない。"

  namespace   = "AWS/ApplicationELB"
  metric_name = "HealthyHostCount"
  dimensions = {
    LoadBalancer = var.alb_arn_suffix
    TargetGroup  = var.web_target_group_arn_suffix
  }
  statistic           = "Minimum"
  period              = 60
  evaluation_periods  = 3
  comparison_operator = "LessThanThreshold"
  threshold           = 1
  treat_missing_data  = "breaching"

  alarm_actions = local.alarm_actions
  ok_actions    = local.alarm_actions
}

resource "aws_cloudwatch_metric_alarm" "web_5xx" {
  count = var.web_attached ? 1 : 0

  alarm_name        = "${var.name_prefix}-web-5xx"
  alarm_description = "フロントエンドのタスクが返した 5xx が 5 分間で ${var.alb_5xx_threshold} 件を超えた。"

  namespace   = "AWS/ApplicationELB"
  metric_name = "HTTPCode_Target_5XX_Count"
  dimensions = {
    LoadBalancer = var.alb_arn_suffix
    TargetGroup  = var.web_target_group_arn_suffix
  }
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  comparison_operator = "GreaterThanThreshold"
  threshold           = var.alb_5xx_threshold
  treat_missing_data  = "notBreaching"

  alarm_actions = local.alarm_actions
  ok_actions    = local.alarm_actions
}

# --- RDS ---

resource "aws_cloudwatch_metric_alarm" "db_cpu" {
  alarm_name        = "${var.name_prefix}-db-cpu"
  alarm_description = "RDS の CPU 使用率が 15 分続けて ${var.db_cpu_threshold}% を超えている。T 系のインスタンスは CPU クレジットを使い切ると追加料金がかかる。"

  namespace           = "AWS/RDS"
  metric_name         = "CPUUtilization"
  dimensions          = { DBInstanceIdentifier = var.db_instance_identifier }
  statistic           = "Average"
  period              = 300
  evaluation_periods  = 3
  comparison_operator = "GreaterThanThreshold"
  threshold           = var.db_cpu_threshold
  treat_missing_data  = "missing"

  alarm_actions = local.alarm_actions
  ok_actions    = local.alarm_actions
}

resource "aws_cloudwatch_metric_alarm" "db_connections" {
  alarm_name        = "${var.name_prefix}-db-connections"
  alarm_description = "RDS への接続数が ${var.db_connections_threshold} を超えた。"

  namespace           = "AWS/RDS"
  metric_name         = "DatabaseConnections"
  dimensions          = { DBInstanceIdentifier = var.db_instance_identifier }
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 1
  comparison_operator = "GreaterThanThreshold"
  threshold           = var.db_connections_threshold
  treat_missing_data  = "missing"

  alarm_actions = local.alarm_actions
  ok_actions    = local.alarm_actions
}

# --- ElastiCache ---

resource "aws_cloudwatch_metric_alarm" "redis_memory" {
  alarm_name        = "${var.name_prefix}-redis-memory"
  alarm_description = "Redis のメモリ使用率が ${var.redis_memory_threshold}% を超えた。使い切るとセッション・マッチングキュー・接続のリースへの書き込みが失敗する。"

  namespace           = "AWS/ElastiCache"
  metric_name         = "DatabaseMemoryUsagePercentage"
  dimensions          = { CacheClusterId = var.redis_cluster_id }
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 1
  comparison_operator = "GreaterThanThreshold"
  threshold           = var.redis_memory_threshold
  treat_missing_data  = "missing"

  alarm_actions = local.alarm_actions
  ok_actions    = local.alarm_actions
}

# --- NAT インスタンス ---

# NAT インスタンスは 1 台だけで、止まるとタスクの起動（イメージの取得・秘密の読み出し）と
# 外向きの通信ができなくなる。
resource "aws_cloudwatch_metric_alarm" "nat_status" {
  alarm_name        = "${var.name_prefix}-nat-status-check"
  alarm_description = "NAT インスタンスのステータスチェックが 2 分続けて失敗している。"

  namespace           = "AWS/EC2"
  metric_name         = "StatusCheckFailed"
  dimensions          = { InstanceId = var.nat_instance_id }
  statistic           = "Maximum"
  period              = 60
  evaluation_periods  = 2
  comparison_operator = "GreaterThanOrEqualToThreshold"
  threshold           = 1
  treat_missing_data  = "breaching"

  alarm_actions = local.alarm_actions
  ok_actions    = local.alarm_actions
}

# --- retention バッチ ---

resource "aws_cloudwatch_log_metric_filter" "retention_failed" {
  count = var.retention_log_group_name == null ? 0 : 1

  name           = "${var.name_prefix}-retention-failed"
  log_group_name = var.retention_log_group_name
  pattern        = "{ $.msg = \"retention failed\" }"

  metric_transformation {
    namespace = "anontopic/retention"
    name      = "${var.name_prefix}-retention-failed"
    value     = "1"
    unit      = "Count"
  }
}

resource "aws_cloudwatch_metric_alarm" "retention_failed" {
  count = var.retention_log_group_name == null ? 0 : 1

  alarm_name        = "${var.name_prefix}-retention-failed"
  alarm_description = "retention バッチが失敗した。会話ログの削除やパーティションの作成が止まっている。"

  namespace           = aws_cloudwatch_log_metric_filter.retention_failed[0].metric_transformation[0].namespace
  metric_name         = aws_cloudwatch_log_metric_filter.retention_failed[0].metric_transformation[0].name
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  threshold           = 1
  treat_missing_data  = "notBreaching"

  alarm_actions = local.alarm_actions
}
