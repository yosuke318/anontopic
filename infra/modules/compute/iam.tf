data "aws_caller_identity" "current" {}

data "aws_region" "current" {}

data "aws_partition" "current" {}

# タスクを起動するときに ECS が使うロール。イメージの取得・ログの送信・秘密の読み出しに使う。
# アプリ自身は AWS の API を呼ばないため、タスクロールは付けない。
resource "aws_iam_role" "execution" {
  name = "${var.name_prefix}-api-execution"

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

# SecureString は AWS 管理キー（aws/ssm）で暗号化する前提で、KMS の権限は付けない。
resource "aws_iam_role_policy" "execution_secrets" {
  count = length(var.secret_names) > 0 ? 1 : 0

  name = "read-secrets"
  role = aws_iam_role.execution.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "ssm:GetParameters"
      Resource = "${local.ssm_parameter_arn_prefix}/*"
    }]
  })
}
