# バケット名はアカウント ID を含むためリポジトリに置かない。init のときに
# -backend-config=backend.tfbackend で渡す。
terraform {
  backend "s3" {
    key          = "prod/terraform.tfstate"
    region       = "ap-northeast-1"
    encrypt      = true
    use_lockfile = true
  }
}
