locals {
  https = var.zone_name != null

  ssm_parameter_arn_prefix = "arn:${data.aws_partition.current.partition}:ssm:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:parameter${var.ssm_parameter_path}"

  # ALB は接続元のアドレスを X-Forwarded-For の末尾に足す。タスクには ALB からしか届かないため、
  # 末尾の値を接続元として信用できる。
  environment = merge(var.environment, {
    APP_ADDR                = ":${var.app_port}"
    APP_TRUST_FORWARDED_FOR = "true"
  })
}

resource "aws_cloudwatch_log_group" "api" {
  name              = "/ecs/${var.name_prefix}-api"
  retention_in_days = var.log_retention_days
}

resource "aws_ecs_cluster" "this" {
  name = var.name_prefix

  setting {
    name  = "containerInsights"
    value = "disabled"
  }
}

resource "aws_ecs_task_definition" "api" {
  family                   = "${var.name_prefix}-api"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.cpu
  memory                   = var.memory
  execution_role_arn       = aws_iam_role.execution.arn

  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "ARM64"
  }

  container_definitions = jsonencode([{
    name      = "api"
    image     = "${aws_ecr_repository.api.repository_url}:${var.initial_image_tag}"
    essential = true

    portMappings = [{
      containerPort = var.app_port
      protocol      = "tcp"
    }]

    environment = [for k, v in local.environment : { name = k, value = v }]
    secrets     = [for name in var.secret_names : { name = name, valueFrom = "${local.ssm_parameter_arn_prefix}/${name}" }]

    # SIGTERM が届くのは ALB からの登録解除を待った後になる。サーバーが書き込み待ちの
    # メッセージを記録し終えるまで（shutdownTimeout、10 秒）待てる長さにする。
    stopTimeout = 30

    readonlyRootFilesystem = true

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.api.name
        awslogs-region        = data.aws_region.current.region
        awslogs-stream-prefix = "api"
      }
    }
  }])
}

resource "aws_ecs_service" "api" {
  name            = "${var.name_prefix}-api"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.api.arn
  desired_count   = var.desired_count
  launch_type     = "FARGATE"

  # 新しいタスクがすべてヘルスチェックを通ってから古いタスクを止める。
  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  network_configuration {
    subnets          = var.private_subnet_ids
    security_groups  = [var.app_security_group_id]
    assign_public_ip = false
  }

  health_check_grace_period_seconds = local.https ? 30 : null

  dynamic "load_balancer" {
    for_each = local.https ? [1] : []

    content {
      target_group_arn = aws_lb_target_group.api.arn
      container_name   = "api"
      container_port   = var.app_port
    }
  }

  propagate_tags = "SERVICE"

  # デプロイはタスク定義の新しいリビジョンを登録してサービスを更新する。Terraform は
  # サービスが使うリビジョンを追わない。
  lifecycle {
    ignore_changes = [task_definition]
  }

  # ターゲットグループがリスナーにつながる前にサービスを作ると失敗する。
  depends_on = [aws_lb_listener.https]
}
