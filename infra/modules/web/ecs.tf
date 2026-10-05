data "aws_caller_identity" "current" {}

data "aws_region" "current" {}

data "aws_partition" "current" {}

resource "aws_cloudwatch_log_group" "web" {
  name              = "/ecs/${var.name_prefix}-web"
  retention_in_days = var.log_retention_days
}

# タスクを起動するときに ECS が使うロール。イメージの取得とログの送信に使う。
# フロントエンドは秘密を持たず、AWS の API も呼ばないため、それ以外の権限とタスクロールは付けない。
resource "aws_iam_role" "execution" {
  name = "${var.name_prefix}-web-execution"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
      Action    = "sts:AssumeRole"
      Condition = {
        StringEquals = { "aws:SourceAccount" = data.aws_caller_identity.current.account_id }
      }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_ecs_task_definition" "web" {
  family                   = "${var.name_prefix}-web"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.cpu
  memory                   = var.memory
  execution_role_arn       = aws_iam_role.execution.arn

  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "ARM64"
  }

  # NEXT_PUBLIC_ で始まる値はビルドのときにイメージへ埋め込むため、ここでは渡さない。
  container_definitions = jsonencode([{
    name      = "web"
    image     = "${aws_ecr_repository.web.repository_url}:${var.initial_image_tag}"
    essential = true

    portMappings = [{
      containerPort = var.port
      protocol      = "tcp"
    }]

    environment = [
      { name = "PORT", value = tostring(var.port) },
      { name = "API_BASE_URL", value = var.api_base_url },
    ]

    readonlyRootFilesystem = true

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.web.name
        awslogs-region        = data.aws_region.current.region
        awslogs-stream-prefix = "web"
      }
    }
  }])
}

resource "aws_lb_target_group" "web" {
  name        = "${var.name_prefix}-web"
  vpc_id      = var.vpc_id
  target_type = "ip"
  protocol    = "HTTP"
  port        = var.port

  deregistration_delay = var.deregistration_delay

  # LP はビルド時に生成した HTML を返すだけで、API に依存しない。
  health_check {
    path                = "/"
    matcher             = "200"
    interval            = 15
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }
}

# CloudFront はオリジンへの要求に local.origin_route_header を付ける。ALB はこのヘッダーが
# ある要求だけをフロントエンドに振り分け、それ以外は既定の動作どおり API に送る。
resource "aws_lb_listener_rule" "web" {
  listener_arn = var.listener_arn
  priority     = var.listener_rule_priority

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.web.arn
  }

  condition {
    http_header {
      http_header_name = local.origin_route_header.name
      values           = [local.origin_route_header.value]
    }
  }
}

resource "aws_ecs_service" "web" {
  name            = "${var.name_prefix}-web"
  cluster         = var.cluster_arn
  task_definition = aws_ecs_task_definition.web.arn
  desired_count   = var.desired_count
  launch_type     = "FARGATE"

  # 新しいタスクがヘルスチェックを通ってから古いタスクを止める。
  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  network_configuration {
    subnets          = var.private_subnet_ids
    security_groups  = [var.security_group_id]
    assign_public_ip = false
  }

  health_check_grace_period_seconds = 30

  load_balancer {
    target_group_arn = aws_lb_target_group.web.arn
    container_name   = "web"
    container_port   = var.port
  }

  propagate_tags = "SERVICE"

  # デプロイはタスク定義の新しいリビジョンを登録してサービスを更新する。Terraform は
  # サービスが使うリビジョンを追わない。
  lifecycle {
    ignore_changes = [task_definition]
  }

  # ターゲットグループがリスナーにつながる前にサービスを作ると失敗する。
  depends_on = [aws_lb_listener_rule.web]
}
