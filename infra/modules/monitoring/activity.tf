# 利用者が来たことを運用者に知らせるアラーム。サーバーは 10 秒ごとに、待機中と会話中の
# 人数の合計（users）と、人数が 0 に戻ってから届いた最大の人数（peak）をログに書く。
# peak を見るため、10 人を一度超えたあとに人数が増減しても、0 に戻るまでは鳴り直さない。
# 理由は docs/adr/0046-mail-the-operator-from-the-peak-number-of-users-written-to-the-log.md。
#
# 知らせたいのは人数が届いたときだけなので、状態が戻ったときの通知（ok_actions）は付けない。

resource "aws_cloudwatch_log_metric_filter" "activity_peak" {
  count = var.activity_log_group_name == null ? 0 : 1

  name           = "${var.name_prefix}-activity-peak"
  log_group_name = var.activity_log_group_name
  pattern        = "{ $.msg = \"activity\" }"

  metric_transformation {
    namespace = "anontopic/activity"
    name      = "${var.name_prefix}-activity-peak"
    value     = "$.peak"
    unit      = "Count"
  }
}

resource "aws_cloudwatch_metric_alarm" "activity_reached" {
  for_each = var.activity_log_group_name == null ? toset([]) : toset([for n in var.activity_thresholds : tostring(n)])

  alarm_name        = "${var.name_prefix}-activity-reached-${each.key}"
  alarm_description = "待機中と会話中の人数の合計が ${each.key} 人に届いた。人数が 0 に戻るまで、この人数ではもう通知しない。"

  namespace           = aws_cloudwatch_log_metric_filter.activity_peak[0].metric_transformation[0].namespace
  metric_name         = aws_cloudwatch_log_metric_filter.activity_peak[0].metric_transformation[0].name
  statistic           = "Maximum"
  period              = 60
  evaluation_periods  = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  threshold           = tonumber(each.key)
  # デプロイでタスクが入れ替わる間はログが途切れる。直前の状態を保ち、途切れたあとに
  # 同じ人数でもう一度鳴らないようにする。
  treat_missing_data = "ignore"

  alarm_actions = local.alarm_actions
}
