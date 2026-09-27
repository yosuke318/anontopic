# GitHub の OIDC プロバイダはアカウントに 1 つしか作れず、同じアカウントの他のリポジトリも
# 使っている。この構成では作らず、既存のものを参照する。
data "aws_iam_openid_connect_provider" "github" {
  url = "https://token.actions.githubusercontent.com"
}

# plan 用。pull request と main への push から引き受けられる。
# IAM が条件に使える GitHub のクレームは aud と sub などに限られ、repository や event_name は
# 使えない。リポジトリと起動のきっかけは sub だけで絞る。
data "aws_iam_policy_document" "terraform_plan_trust" {
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
      values = flatten([
        for prefix in local.github_sub_prefixes : [
          "${prefix}:pull_request",
          "${prefix}:ref:refs/heads/main",
        ]
      ])
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
      values = flatten([
        for prefix in local.github_sub_prefixes : [
          "${prefix}:environment:dev",
          "${prefix}:environment:prod",
        ]
      ])
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
