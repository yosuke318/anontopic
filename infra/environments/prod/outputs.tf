output "ecr_repository_url" {
  value = module.compute.ecr_repository_url
}

output "ecs_cluster_name" {
  value = module.compute.cluster_name
}

output "ecs_service_name" {
  value = module.compute.service_name
}

output "api_url" {
  value = module.compute.api_url
}

output "nat_instance_id" {
  value = module.network.nat_instance_id
}

output "database_address" {
  value = module.database.address
}

output "database_name" {
  value = module.database.database_name
}

output "database_master_user_secret_arn" {
  value = module.database.master_user_secret_arn
}

output "redis_address" {
  value = module.cache.address
}

output "alerts_topic_arn" {
  value = module.monitoring.alerts_topic_arn
}
