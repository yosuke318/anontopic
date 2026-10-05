locals {
  # ALB のリスナールールが見るヘッダー。フロントエンドは公開しているページしか返さないため、
  # ヘッダーを知っている人が ALB に直接送っても、CloudFront を通したときと同じものが返るだけになる。
  origin_route_header = {
    name  = "X-Anontopic-Route"
    value = "web"
  }

  origin_id = "alb"
}

# ホストゾーンはこのモジュールでは作らず、ドメインを取得したときに作られたものを参照する。
data "aws_route53_zone" "this" {
  name         = var.zone_name
  private_zone = false
}

resource "aws_acm_certificate" "site" {
  provider = aws.us_east_1

  domain_name       = var.domain_name
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

# 証明書の対象はサイトのドメイン 1 つだけなので、検証用のレコードも 1 つになる。
resource "aws_route53_record" "site_validation" {
  zone_id         = data.aws_route53_zone.this.zone_id
  name            = one(aws_acm_certificate.site.domain_validation_options).resource_record_name
  type            = one(aws_acm_certificate.site.domain_validation_options).resource_record_type
  records         = [one(aws_acm_certificate.site.domain_validation_options).resource_record_value]
  ttl             = 300
  allow_overwrite = true
}

resource "aws_acm_certificate_validation" "site" {
  provider = aws.us_east_1

  certificate_arn         = aws_acm_certificate.site.arn
  validation_record_fqdns = [aws_route53_record.site_validation.fqdn]
}

data "aws_cloudfront_cache_policy" "caching_disabled" {
  name = "Managed-CachingDisabled"
}

data "aws_cloudfront_cache_policy" "caching_optimized" {
  name = "Managed-CachingOptimized"
}

# Host はオリジンのドメインのまま送る。ALB の証明書と照合されるのはこの名前になる。
data "aws_cloudfront_origin_request_policy" "all_viewer_except_host_header" {
  name = "Managed-AllViewerExceptHostHeader"
}

resource "aws_cloudfront_distribution" "site" {
  enabled         = true
  is_ipv6_enabled = true
  http_version    = "http2and3"
  price_class     = var.price_class
  aliases         = [var.domain_name]
  comment         = "${var.name_prefix} web"

  origin {
    origin_id   = local.origin_id
    domain_name = var.origin_domain_name

    custom_origin_config {
      http_port              = 80
      https_port             = 443
      origin_protocol_policy = "https-only"
      origin_ssl_protocols   = ["TLSv1.2"]
    }

    custom_header {
      name  = local.origin_route_header.name
      value = local.origin_route_header.value
    }
  }

  # HTML は CloudFront に持たせず、毎回タスクが返す。/topics と /waiting はリクエストごとに
  # 描画し（ADR-0014）、LP と紹介ページもデプロイのたびに消し直さずに済む。
  default_cache_behavior {
    target_origin_id         = local.origin_id
    viewer_protocol_policy   = "redirect-to-https"
    allowed_methods          = ["GET", "HEAD", "OPTIONS"]
    cached_methods           = ["GET", "HEAD"]
    compress                 = true
    cache_policy_id          = data.aws_cloudfront_cache_policy.caching_disabled.id
    origin_request_policy_id = data.aws_cloudfront_origin_request_policy.all_viewer_except_host_header.id
  }

  # ビルドが出す JS・CSS はファイル名に内容のハッシュを含み、Next.js も immutable で返す。
  ordered_cache_behavior {
    path_pattern           = "/_next/static/*"
    target_origin_id       = local.origin_id
    viewer_protocol_policy = "redirect-to-https"
    allowed_methods        = ["GET", "HEAD"]
    cached_methods         = ["GET", "HEAD"]
    compress               = true
    cache_policy_id        = data.aws_cloudfront_cache_policy.caching_optimized.id
  }

  # デプロイ中は新旧のタスクが並び、新しい HTML が指すファイルを古いタスクが 404 で返すことがある。
  # その 404 をエッジに残さない。
  custom_error_response {
    error_code            = 404
    error_caching_min_ttl = 0
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    acm_certificate_arn      = aws_acm_certificate_validation.site.certificate_arn
    ssl_support_method       = "sni-only"
    minimum_protocol_version = "TLSv1.2_2021"
  }
}

resource "aws_route53_record" "site" {
  for_each = toset(["A", "AAAA"])

  zone_id = data.aws_route53_zone.this.zone_id
  name    = var.domain_name
  type    = each.value

  alias {
    name                   = aws_cloudfront_distribution.site.domain_name
    zone_id                = aws_cloudfront_distribution.site.hosted_zone_id
    evaluate_target_health = false
  }
}
