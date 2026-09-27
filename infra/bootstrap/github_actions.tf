# GitHub の OIDC プロバイダはアカウントに 1 つしか作れず、同じアカウントの他のリポジトリも
# 使っている。この構成では作らず、既存のものを参照する。
data "aws_iam_openid_connect_provider" "github" {
  url = "https://token.actions.githubusercontent.com"
}

# plan 用。pull request と main への push から引き受けられる。
#
# リポジトリの絞り込みは repository / event_name / ref クレームで行う。GitHub は組織や
# リポジトリの名前が変わったことを検知すると、sub を "repo:owner@<owner_id>/repo@<repo_id>:..."
# という ID 付きの形式に変えるが、repository クレームは常に "owner/repo" のままなので、
# この揺れの影響を受けない。sub の条件は、AWS が GitHub の OIDC プロバイダに対して
# 要求する「sub か job_workflow_ref による絞り込み」を満たすためだけに、ID の有無どちらにも
# 一致するワイルドカードで残す。
data "aws_iam_policy_document" "terraform_plan_trust" {
  statement {
    sid     = "PullRequest"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [data.aws_iam_openid_connect_provider.github.arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringLike"
      variable = "token.actions.githubusercontent.com:sub"
      values   = [local.github_sub_pattern]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:repository"
      values   = [var.github_repository]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:event_name"
      values   = ["pull_request"]
    }
  }

  statement {
    sid     = "PushToMain"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [data.aws_iam_openid_connect_provider.github.arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringLike"
      variable = "token.actions.githubusercontent.com:sub"
      values   = [local.github_sub_pattern]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:repository"
      values   = [var.github_repository]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:event_name"
      values   = ["push"]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:ref"
      values   = ["refs/heads/main"]
    }
  }
}

resource "aws_iam_role" "terraform_plan" {
  name                 = "${local.project}-terraform-plan"
  assume_role_policy   = data.aws_iam_policy_document.terraform_plan_trust.json
  max_session_duration = 3600
}

resource "aws_iam_role_policy_attachment" "terraform_plan_read_only" {
  role       = aws_iam_role.terraform_plan.name
  policy_arn = "arn:aws:iam::aws:policy/ReadOnlyAccess"
}

# S3 のネイティブロックは state の隣にロックファイルを書くため、読み取り専用のままでは plan が取れない。
data "aws_iam_policy_document" "terraform_plan_state_lock" {
  statement {
    actions   = ["s3:PutObject", "s3:DeleteObject"]
    resources = ["${aws_s3_bucket.tfstate.arn}/*/terraform.tfstate.tflock"]
  }
}

resource "aws_iam_role_policy" "terraform_plan_state_lock" {
  name   = "state-lock"
  role   = aws_iam_role.terraform_plan.id
  policy = data.aws_iam_policy_document.terraform_plan_state_lock.json
}

# apply 用。GitHub の Environment（dev / prod）を指定したジョブからだけ引き受けられる。
# どのブランチから、誰の承認で apply できるかは Environment の保護ルールで絞る。
# plan 用と同じ理由で、リポジトリの絞り込みは repository クレームで行い、sub は
# AWS が要求する絞り込み条件を満たすためだけにワイルドカードで残す。
data "aws_iam_policy_document" "terraform_apply_trust" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [data.aws_iam_openid_connect_provider.github.arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringLike"
      variable = "token.actions.githubusercontent.com:sub"
      values   = [local.github_sub_pattern]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:repository"
      values   = [var.github_repository]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:environment"
      values   = ["dev", "prod"]
    }
  }
}

resource "aws_iam_role" "terraform_apply" {
  name                 = "${local.project}-terraform-apply"
  assume_role_policy   = data.aws_iam_policy_document.terraform_apply_trust.json
  max_session_duration = 3600
}

# IAM ロールやポリシーも Terraform で作るため、権限を絞りきれない。
resource "aws_iam_role_policy_attachment" "terraform_apply_admin" {
  role       = aws_iam_role.terraform_apply.name
  policy_arn = "arn:aws:iam::aws:policy/AdministratorAccess"
}
