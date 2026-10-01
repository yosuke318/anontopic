module "network" {
  source = "../../modules/network"

  name_prefix        = "${local.project}-${local.env}"
  vpc_cidr           = "10.1.0.0/16"
  availability_zones = ["ap-northeast-1a", "ap-northeast-1c"]
}
