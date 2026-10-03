resource "aws_lb" "api" {
  name               = "${var.name_prefix}-api"
  load_balancer_type = "application"
  internal           = false
  subnets            = var.public_subnet_ids
  security_groups    = [var.alb_security_group_id]

  idle_timeout               = var.idle_timeout
  drop_invalid_header_fields = true
}

# アプリの状態は Redis と PostgreSQL にあり、同じ会話の参加者が別々のタスクにつないでも
# メッセージは届く。そのためスティッキーセッションは使わない。
resource "aws_lb_target_group" "api" {
  name        = "${var.name_prefix}-api"
  vpc_id      = var.vpc_id
  target_type = "ip"
  protocol    = "HTTP"
  port        = var.app_port

  deregistration_delay = var.deregistration_delay

  # /healthz はプロセスが動いているかだけを返す。PostgreSQL や Redis が止まったときに
  # すべてのタスクが外れないよう、/readyz は使わない。
  health_check {
    path                = "/healthz"
    matcher             = "200"
    interval            = 15
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }
}

resource "aws_lb_listener" "https" {
  count = local.https ? 1 : 0

  load_balancer_arn = aws_lb.api.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = aws_acm_certificate_validation.api[0].certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api.arn
  }
}
