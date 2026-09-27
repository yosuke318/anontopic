locals {
  project = "anontopic"
  region  = "ap-northeast-1"

  # GitHub の OIDC トークンの sub は "repo:<owner>/<repo>:<文脈>" の形をとる。組織や
  # リポジトリの名前が変わったことがある場合は "repo:<owner>@<owner_id>/<repo>@<repo_id>:<文脈>"
  # になる。どちらの形でも一致するよう、両方の接頭辞を持っておく。
  # ユーザー名とリポジトリ名には "@" を使えないため、"@*" は ID の部分にしか一致しない。
  github_repository_owner = split("/", var.github_repository)[0]
  github_repository_name  = split("/", var.github_repository)[1]
  github_sub_prefixes = [
    "repo:${var.github_repository}",
    "repo:${local.github_repository_owner}@*/${local.github_repository_name}@*",
  ]
}
