provider "aws" {
  region              = local.region
  allowed_account_ids = [var.aws_account_id]

  default_tags {
    tags = {
      Project   = local.project
      Env       = local.env
      ManagedBy = "terraform"
    }
  }
}

# CloudFront に付ける ACM の証明書を置くリージョン。
provider "aws" {
  alias               = "us_east_1"
  region              = "us-east-1"
  allowed_account_ids = [var.aws_account_id]

  default_tags {
    tags = {
      Project   = local.project
      Env       = local.env
      ManagedBy = "terraform"
    }
  }
}
