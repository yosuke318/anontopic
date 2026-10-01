# ECR のイメージのレイヤーは S3 から取得する。ゲートウェイ型は無料なので常に作り、
# イメージの取得を NAT インスタンスに通さない。
resource "aws_vpc_endpoint" "s3" {
  vpc_id            = aws_vpc.this.id
  service_name      = "com.amazonaws.${data.aws_region.current.region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [aws_route_table.private.id]

  tags = {
    Name = "${var.name_prefix}-s3"
  }
}

resource "aws_vpc_endpoint" "interface" {
  for_each = toset(var.interface_endpoint_services)

  vpc_id              = aws_vpc.this.id
  service_name        = "com.amazonaws.${data.aws_region.current.region}.${each.key}"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = aws_subnet.private[*].id
  security_group_ids  = [aws_security_group.endpoint[0].id]
  private_dns_enabled = true

  tags = {
    Name = "${var.name_prefix}-${each.key}"
  }
}

resource "aws_security_group" "endpoint" {
  count = length(var.interface_endpoint_services) > 0 ? 1 : 0

  name        = "${var.name_prefix}-endpoint"
  description = "Interface VPC endpoints: HTTPS from the app"
  vpc_id      = aws_vpc.this.id

  tags = {
    Name = "${var.name_prefix}-endpoint"
  }
}

resource "aws_vpc_security_group_ingress_rule" "endpoint_from_app" {
  count = length(aws_security_group.endpoint)

  security_group_id            = aws_security_group.endpoint[0].id
  description                  = "HTTPS from the app"
  ip_protocol                  = "tcp"
  from_port                    = 443
  to_port                      = 443
  referenced_security_group_id = aws_security_group.app.id
}
