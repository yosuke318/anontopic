output "ecr_repository_url" {
  value = aws_ecr_repository.web.repository_url
}

output "service_name" {
  value = aws_ecs_service.web.name
}

output "task_definition_family" {
  value = aws_ecs_task_definition.web.family
}

output "target_group_arn_suffix" {
  description = "ターゲットグループの ARN の末尾。CloudWatch のメトリクスのディメンション TargetGroup に使う。"
  value       = aws_lb_target_group.web.arn_suffix
}

output "site_url" {
  value = "https://${var.domain_name}"
}

output "distribution_id" {
  value = aws_cloudfront_distribution.site.id
}
