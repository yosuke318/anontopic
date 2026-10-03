output "alerts_topic_arn" {
  description = "アラームと予算の通知を送る SNS トピックの ARN。"
  value       = aws_sns_topic.alerts.arn
}
