output "address" {
  value = aws_elasticache_replication_group.this.primary_endpoint_address
}

output "port" {
  value = aws_elasticache_replication_group.this.port
}

output "cluster_id" {
  description = "レプリケーショングループの最初のノードのクラスター ID。CloudWatch のメトリクスのディメンション CacheClusterId に使う。"
  value       = sort(tolist(aws_elasticache_replication_group.this.member_clusters))[0]
}
