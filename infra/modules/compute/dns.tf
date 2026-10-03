# ホストゾーンはこのモジュールでは作らず、ドメインを取得したときに作られたものを参照する。
data "aws_route53_zone" "this" {
  count = local.https ? 1 : 0

  name         = var.zone_name
  private_zone = false
}

locals {
  api_domain_name = local.https ? "${var.api_record_name}.${var.zone_name}" : null
}

resource "aws_acm_certificate" "api" {
  count = local.https ? 1 : 0

  domain_name       = local.api_domain_name
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

# 証明書の対象は API のドメイン 1 つだけなので、検証用のレコードも 1 つになる。
resource "aws_route53_record" "api_validation" {
  count = local.https ? 1 : 0

  zone_id         = data.aws_route53_zone.this[0].zone_id
  name            = one(aws_acm_certificate.api[0].domain_validation_options).resource_record_name
  type            = one(aws_acm_certificate.api[0].domain_validation_options).resource_record_type
  records         = [one(aws_acm_certificate.api[0].domain_validation_options).resource_record_value]
  ttl             = 300
  allow_overwrite = true
}

resource "aws_acm_certificate_validation" "api" {
  count = local.https ? 1 : 0

  certificate_arn         = aws_acm_certificate.api[0].arn
  validation_record_fqdns = [aws_route53_record.api_validation[0].fqdn]
}

resource "aws_route53_record" "api" {
  count = local.https ? 1 : 0

  zone_id = data.aws_route53_zone.this[0].zone_id
  name    = local.api_domain_name
  type    = "A"

  alias {
    name                   = aws_lb.api.dns_name
    zone_id                = aws_lb.api.zone_id
    evaluate_target_health = false
  }
}
