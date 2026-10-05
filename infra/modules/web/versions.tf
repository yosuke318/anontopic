terraform {
  required_version = "~> 1.15"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.66, < 7.0"

      # CloudFront に付ける ACM の証明書は us-east-1 にしか置けない。
      configuration_aliases = [aws.us_east_1]
    }
  }
}
