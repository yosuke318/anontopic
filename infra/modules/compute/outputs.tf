output "ecr_repository_url" {
  value = aws_ecr_repository.api.repository_url
}

output "cluster_name" {
  value = aws_ecs_cluster.this.name
}

output "service_name" {
  value = aws_ecs_service.api.name
}

output "task_definition_family" {
  value = aws_ecs_task_definition.api.family
}

output "alb_dns_name" {
  value = aws_lb.api.dns_name
}

output "api_url" {
  description = "API の URL。zone_name が null のときは null。"
  value       = local.https ? "https://${local.api_domain_name}" : null
}

output "alb_arn_suffix" {
  description = "ALB の ARN の末尾。CloudWatch のメトリクスのディメンション LoadBalancer に使う。"
  value       = aws_lb.api.arn_suffix
}

output "target_group_arn_suffix" {
  description = "ターゲットグループの ARN の末尾。CloudWatch のメトリクスのディメンション TargetGroup に使う。"
  value       = aws_lb_target_group.api.arn_suffix
}

output "load_balancer_attached" {
  description = "サービスが ALB のターゲットグループにつながっているか。zone_name が null のときは false。"
  value       = local.https
}
