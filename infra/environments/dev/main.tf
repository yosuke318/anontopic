module "network" {
  source = "../../modules/network"

  name_prefix        = "${local.project}-${local.env}"
  vpc_cidr           = "10.0.0.0/16"
  availability_zones = ["ap-northeast-1a", "ap-northeast-1c"]
  app_port           = local.app_port
  web_port           = local.web_port
}

module "compute" {
  source = "../../modules/compute"

  name_prefix           = "${local.project}-${local.env}"
  vpc_id                = module.network.vpc_id
  public_subnet_ids     = module.network.public_subnet_ids
  private_subnet_ids    = module.network.private_subnet_ids
  alb_security_group_id = module.network.alb_security_group_id
  app_security_group_id = module.network.app_security_group_id
  app_port              = local.app_port

  desired_count = 1

  # dev は追加開発のときだけ作り、普段は destroy しておく。
  ecr_force_delete = true
  cpu              = 256
  memory           = 512

  environment = local.domain_name == null ? {} : {
    APP_ALLOWED_ORIGINS = "https://dev.${local.domain_name}"
  }

  ssm_parameter_path = "/${local.project}/${local.env}/api"
  secret_names = [
    "DATABASE_URL",
    "REDIS_URL",
    "SESSION_IP_HASH_SECRET",
    "ADMIN_API_TOKEN",
  ]

  zone_name       = local.domain_name
  api_record_name = "api.dev"
}

# サイトは CloudFront から ALB を通して配る（ADR-0035）。ドメインが無いと CloudFront に付ける
# 証明書も API のオリジンも無いため、作らない。
module "web" {
  source = "../../modules/web"
  count  = local.domain_name == null ? 0 : 1

  providers = {
    aws           = aws
    aws.us_east_1 = aws.us_east_1
  }

  name_prefix        = "${local.project}-${local.env}"
  vpc_id             = module.network.vpc_id
  private_subnet_ids = module.network.private_subnet_ids
  security_group_id  = module.network.web_security_group_id
  port               = local.web_port

  cluster_arn   = module.compute.cluster_arn
  desired_count = 1

  # dev は追加開発のときだけ作り、普段は destroy しておく。
  ecr_force_delete = true
  cpu              = 256
  memory           = 512

  api_base_url = module.compute.api_url

  listener_arn       = module.compute.https_listener_arn
  zone_name          = local.domain_name
  domain_name        = "dev.${local.domain_name}"
  origin_domain_name = module.compute.api_domain_name
}

module "database" {
  source = "../../modules/database"

  name_prefix       = "${local.project}-${local.env}"
  subnet_ids        = module.network.private_subnet_ids
  security_group_id = module.network.database_security_group_id

  instance_class        = "db.t4g.micro"
  max_allocated_storage = 50
  backup_retention_days = 1
}

module "cache" {
  source = "../../modules/cache"

  name_prefix       = "${local.project}-${local.env}"
  subnet_ids        = module.network.private_subnet_ids
  security_group_id = module.network.cache_security_group_id

  node_type = "cache.t4g.micro"
}
