output "state_bucket_name" {
  value = aws_s3_bucket.tfstate.bucket
}

output "terraform_plan_role_arn" {
  value = aws_iam_role.terraform_plan.arn
}

output "terraform_apply_role_arn" {
  value = aws_iam_role.terraform_apply.arn
}

output "deploy_role_arn" {
  value = aws_iam_role.deploy.arn
}
