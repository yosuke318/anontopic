locals {
  project = "anontopic"
  region  = "ap-northeast-1"

  # AWS は GitHub の OIDC プロバイダを信頼するトラストポリシーに、sub か job_workflow_ref を
  # 条件に含めることを要求する（aud だけでは他のあらゆるリポジトリから引き受けられてしまうため）。
  # sub は組織やリポジトリの名前変更を検知すると owner/repo に @<id> が付く形式に変わるので、
  # StringLike のワイルドカードで両方の形式を受け付ける。
  github_repository_owner = split("/", var.github_repository)[0]
  github_repository_name  = split("/", var.github_repository)[1]
  github_sub_pattern      = "repo:${local.github_repository_owner}*/${local.github_repository_name}*:*"
}
