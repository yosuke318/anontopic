output "vpc_id" {
  value = aws_vpc.this.id
}

output "public_subnet_ids" {
  value = aws_subnet.public[*].id
}

output "private_subnet_ids" {
  value = aws_subnet.private[*].id
}

output "primary_availability_zone" {
  description = "ステートフルなリソース（RDS・ElastiCache）とアプリのタスクを置く AZ。NAT インスタンスと同じ、availability_zones の先頭。"
  value       = var.availability_zones[0]
}

output "primary_private_subnet_id" {
  description = "primary_availability_zone にあるプライベートサブネットの ID。アプリのタスクはここに置く。"
  value       = aws_subnet.private[0].id
}

output "alb_security_group_id" {
  value = aws_security_group.alb.id
}

output "app_security_group_id" {
  value = aws_security_group.app.id
}

output "web_security_group_id" {
  value = aws_security_group.web.id
}

output "database_security_group_id" {
  value = aws_security_group.database.id
}

output "cache_security_group_id" {
  value = aws_security_group.cache.id
}

output "nat_instance_id" {
  description = "NAT インスタンスの ID。運用者が Session Manager のポートフォワードで RDS に入るときの経由先になる。"
  value       = aws_instance.nat.id
}
