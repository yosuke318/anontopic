# アプリのデプロイ用。イメージを ECR に入れ、タスク定義の新しいリビジョンでマイグレーションを流し、
# サービスを切り替える。インフラの変更は terraform-apply ロールが受け持つため、ここでは持たない。
locals {
  arn_prefix = {
    ecr = "arn:aws:ecr:${local.region}:${var.aws_account_id}"
    ecs = "arn:aws:ecs:${local.region}:${var.aws_account_id}"
    iam = "arn:aws:iam::${var.aws_account_id}"
  }
}

resource "aws_iam_role" "deploy" {
  name                 = "${local.project}-deploy"
  assume_role_policy   = data.aws_iam_policy_document.github_environment_trust.json
  max_session_duration = 3600
}

data "aws_iam_policy_document" "deploy" {
  statement {
    sid       = "EcrLogin"
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"]
  }

  statement {
    sid = "EcrPush"
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:BatchGetImage",
      "ecr:CompleteLayerUpload",
      "ecr:DescribeImages",
      "ecr:DescribeRepositories",
      "ecr:InitiateLayerUpload",
      "ecr:PutImage",
      "ecr:UploadLayerPart",
    ]
    resources = ["${local.arn_prefix.ecr}:repository/${local.project}-*-api"]
  }

  # タスク定義の読み出しと登録は、リソースで絞れない。
  statement {
    sid       = "TaskDefinitions"
    actions   = ["ecs:DescribeTaskDefinition", "ecs:RegisterTaskDefinition"]
    resources = ["*"]
  }

  statement {
    sid       = "TagTaskDefinitions"
    actions   = ["ecs:TagResource"]
    resources = ["${local.arn_prefix.ecs}:task-definition/${local.project}-*-api:*"]

    condition {
      test     = "StringEquals"
      variable = "ecs:CreateAction"
      values   = ["RegisterTaskDefinition"]
    }
  }

  statement {
    sid       = "Services"
    actions   = ["ecs:DescribeServices", "ecs:UpdateService"]
    resources = ["${local.arn_prefix.ecs}:service/${local.project}-*/${local.project}-*-api"]
  }

  statement {
    sid       = "Clusters"
    actions   = ["ecs:DescribeClusters"]
    resources = ["${local.arn_prefix.ecs}:cluster/${local.project}-*"]
  }

  statement {
    sid       = "MigrationTasks"
    actions   = ["ecs:RunTask"]
    resources = ["${local.arn_prefix.ecs}:task-definition/${local.project}-*-api:*"]

    condition {
      test     = "ArnLike"
      variable = "ecs:cluster"
      values   = ["${local.arn_prefix.ecs}:cluster/${local.project}-*"]
    }
  }

  statement {
    sid       = "ReadTasks"
    actions   = ["ecs:DescribeTasks"]
    resources = ["${local.arn_prefix.ecs}:task/${local.project}-*/*"]
  }

  # タスク定義の登録と RunTask には、タスクが使う実行ロールを渡す権限が要る。
  statement {
    sid       = "PassExecutionRole"
    actions   = ["iam:PassRole"]
    resources = ["${local.arn_prefix.iam}:role/${local.project}-*-api-execution"]

    condition {
      test     = "StringEquals"
      variable = "iam:PassedToService"
      values   = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role_policy" "deploy" {
  name   = "deploy"
  role   = aws_iam_role.deploy.id
  policy = data.aws_iam_policy_document.deploy.json
}
