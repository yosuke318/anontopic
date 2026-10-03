output "address" {
  value = aws_db_instance.this.address
}

output "port" {
  value = aws_db_instance.this.port
}

output "database_name" {
  value = aws_db_instance.this.db_name
}

output "master_user_secret_arn" {
  description = "マスターユーザーのパスワードを持つ Secrets Manager のシークレットの ARN。"
  value       = aws_db_instance.this.master_user_secret[0].secret_arn
}

output "identifier" {
  value = aws_db_instance.this.identifier
}
