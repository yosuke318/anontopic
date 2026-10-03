module "network" {
  source = "../../modules/network"

  name_prefix        = "${local.project}-${local.env}"
  vpc_cidr           = "10.1.0.0/16"
  availability_zones = ["ap-northeast-1a", "ap-northeast-1c"]
  app_port           = local.app_port
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

  desired_count = 2
  cpu           = 256
  memory        = 512

  environment = local.domain_name == null ? {} : {
    APP_ALLOWED_ORIGINS = "https://${local.domain_name}"
  }

  ssm_parameter_path = "/${local.project}/${local.env}/api"
  secret_names = [
    "DATABASE_URL",
    "REDIS_URL",
    "SESSION_IP_HASH_SECRET",
    "ADMIN_API_TOKEN",
  ]

  zone_name       = local.domain_name
  api_record_name = "api"
}

module "database" {
  source = "../../modules/database"

  name_prefix       = "${local.project}-${local.env}"
  subnet_ids        = module.network.private_subnet_ids
  security_group_id = module.network.database_security_group_id

  instance_class        = "db.t4g.small"
  max_allocated_storage = 300
  backup_retention_days = 7
}

module "cache" {
  source = "../../modules/cache"

  name_prefix       = "${local.project}-${local.env}"
  subnet_ids        = module.network.private_subnet_ids
  security_group_id = module.network.cache_security_group_id

  node_type = "cache.t4g.micro"
}

# dev は追加開発のときだけ動かすため、アラームと予算は prod にだけ置く。
module "monitoring" {
  source = "../../modules/monitoring"

  name_prefix = "${local.project}-${local.env}"
  alarm_email = local.alarm_email

  # 予算はアカウント全体（dev を含む）の費用を見る。月 4 万円前後を USD に直した値。
  budget_name        = "${local.project}-account-monthly"
  monthly_budget_usd = 270

  ecs_cluster_name        = module.compute.cluster_name
  ecs_service_name        = module.compute.service_name
  load_balancer_attached  = module.compute.load_balancer_attached
  alb_arn_suffix          = module.compute.alb_arn_suffix
  target_group_arn_suffix = module.compute.target_group_arn_suffix
  db_instance_identifier  = module.database.identifier
  redis_cluster_id        = module.cache.cluster_id
  nat_instance_id         = module.network.nat_instance_id
}
