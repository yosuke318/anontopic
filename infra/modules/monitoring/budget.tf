# 予算はアカウント全体の費用を対象にし、dev や state バケットの費用も含める。
# 費用のデータは 1 日に数回しか更新されないため、通知は超えてから数時間遅れて届くことがある。
resource "aws_budgets_budget" "monthly" {
  count = var.budget_name == null ? 0 : 1

  name         = var.budget_name
  budget_type  = "COST"
  time_unit    = "MONTHLY"
  limit_amount = tostring(var.monthly_budget_usd)
  limit_unit   = "USD"

  dynamic "notification" {
    for_each = [50, 80, 100]

    content {
      comparison_operator       = "GREATER_THAN"
      threshold                 = notification.value
      threshold_type            = "PERCENTAGE"
      notification_type         = "ACTUAL"
      subscriber_sns_topic_arns = [aws_sns_topic.alerts.arn]
    }
  }

  # トピックのポリシーで Budgets からの送信を許してから、予算に通知先として登録する。
  depends_on = [aws_sns_topic_policy.alerts]

  lifecycle {
    precondition {
      condition     = var.monthly_budget_usd != null
      error_message = "budget_name を指定するときは monthly_budget_usd も指定する。"
    }
  }
}
