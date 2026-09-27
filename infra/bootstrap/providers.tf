provider "aws" {
  region              = local.region
  allowed_account_ids = [var.aws_account_id]

  default_tags {
    tags = {
      Project   = local.project
      Env       = "shared"
      ManagedBy = "terraform"
    }
  }
}
