# retention バッチ（cmd/retention）を毎日 1 回、ECS のタスクとして動かす（ADR-0041）。
# API のタスク定義をリビジョンを指定せずに使い、コマンドだけを /retention に替える。
# ECS はファミリーの最新のリビジョンを選ぶため、デプロイが登録したイメージで動く。

locals {
  task_definition_family_arn = "arn:${data.aws_partition.current.partition}:ecs:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:task-definition/${aws_ecs_task_definition.api.family}"
}

resource "aws_iam_role" "retention_scheduler" {
  name = "${var.name_prefix}-retention-scheduler"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "scheduler.amazonaws.com" }
      Action    = "sts:AssumeRole"
      Condition = {
        StringEquals = { "aws:SourceAccount" = data.aws_caller_identity.current.account_id }
      }
    }]
  })
}

resource "aws_iam_role_policy" "retention_scheduler" {
  name = "run-retention-task"
  role = aws_iam_role.retention_scheduler.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = "ecs:RunTask"
        Resource = [local.task_definition_family_arn, "${local.task_definition_family_arn}:*"]
        Condition = {
          ArnEquals = { "ecs:cluster" = aws_ecs_cluster.this.arn }
        }
      },
      {
        Effect   = "Allow"
        Action   = "iam:PassRole"
        Resource = aws_iam_role.execution.arn
        Condition = {
          StringEquals = { "iam:PassedToService" = "ecs-tasks.amazonaws.com" }
        }
      },
    ]
  })
}

resource "aws_scheduler_schedule" "retention" {
  name        = "${var.name_prefix}-retention"
  description = "Runs cmd/retention once a day"

  schedule_expression          = var.retention_schedule
  schedule_expression_timezone = "Asia/Tokyo"

  flexible_time_window {
    mode = "OFF"
  }

  # テンプレートの ECS ターゲットはタスク定義の ARN を求め、コマンドを差し替えられないため、
  # RunTask をそのまま呼ぶユニバーサルターゲットを使う。入力は RunTask の要求の形で書く。
  target {
    arn      = "arn:${data.aws_partition.current.partition}:scheduler:::aws-sdk:ecs:runTask"
    role_arn = aws_iam_role.retention_scheduler.arn

    input = jsonencode({
      Cluster        = aws_ecs_cluster.this.arn
      TaskDefinition = aws_ecs_task_definition.api.family
      LaunchType     = "FARGATE"
      StartedBy      = "scheduler-retention"
      NetworkConfiguration = {
        AwsvpcConfiguration = {
          Subnets        = var.private_subnet_ids
          SecurityGroups = [var.app_security_group_id]
          AssignPublicIp = "DISABLED"
        }
      }
      Overrides = {
        ContainerOverrides = [{
          Name    = "api"
          Command = ["/retention"]
        }]
      }
    })

    # 起動の要求が一時的に失敗したときに、1 時間までやり直す。
    retry_policy {
      maximum_event_age_in_seconds = 3600
      maximum_retry_attempts       = 3
    }
  }

  depends_on = [aws_iam_role_policy.retention_scheduler]
}
