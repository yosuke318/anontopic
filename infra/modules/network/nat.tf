# プライベートサブネットからインターネットへ出る通信は、パブリックサブネットに置いた
# NAT インスタンスで送信元アドレスを変換する（ADR-0028）。

data "aws_ssm_parameter" "al2023_arm64" {
  name = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64"
}

resource "aws_security_group" "nat" {
  name        = "${var.name_prefix}-nat"
  description = "NAT instance: forwards outbound traffic from the private subnets"
  vpc_id      = aws_vpc.this.id

  tags = {
    Name = "${var.name_prefix}-nat"
  }
}

resource "aws_vpc_security_group_ingress_rule" "nat_from_private" {
  count = length(aws_subnet.private)

  security_group_id = aws_security_group.nat.id
  description       = "All traffic from ${aws_subnet.private[count.index].tags.Name}"
  ip_protocol       = "-1"
  cidr_ipv4         = aws_subnet.private[count.index].cidr_block
}

resource "aws_vpc_security_group_egress_rule" "nat_to_internet" {
  security_group_id = aws_security_group.nat.id
  description       = "All traffic to the internet"
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

# SSH は開けず、調査が要るときは Session Manager で入る。
resource "aws_iam_role" "nat" {
  name = "${var.name_prefix}-nat-instance"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "nat_ssm" {
  role       = aws_iam_role.nat.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "nat" {
  name = "${var.name_prefix}-nat-instance"
  role = aws_iam_role.nat.name
}

resource "aws_instance" "nat" {
  ami                         = data.aws_ssm_parameter.al2023_arm64.insecure_value
  instance_type               = var.nat_instance_type
  subnet_id                   = aws_subnet.public[0].id
  vpc_security_group_ids      = [aws_security_group.nat.id]
  iam_instance_profile        = aws_iam_instance_profile.nat.name
  associate_public_ip_address = true

  # 自分宛てでないパケットを転送するため、送信元 / 送信先チェックを外す。
  source_dest_check = false

  user_data                   = file("${path.module}/nat_user_data.sh")
  user_data_replace_on_change = true

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  root_block_device {
    volume_type = "gp3"
    encrypted   = true
  }

  tags = {
    Name = "${var.name_prefix}-nat"
  }

  # AMI の最新版が出るたびに作り直さない。更新するときは -replace で入れ替える。
  lifecycle {
    ignore_changes = [ami]
  }
}
