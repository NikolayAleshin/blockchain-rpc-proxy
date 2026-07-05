provider "aws" {
  region = var.aws_region
}

data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  azs = slice(data.aws_availability_zones.available.names, 0, 2)

  tags = {
    Project     = var.name
    Environment = var.environment
    Owner       = var.owner
    CostCenter  = var.cost_center
    ManagedBy   = "opentofu"
  }
}

module "network" {
  source = "../../modules/network"

  name                 = "${var.name}-${var.environment}"
  azs                  = local.azs
  enable_nat           = var.enable_nat
  enable_vpc_endpoints = var.enable_vpc_endpoints
  tags                 = local.tags
}

module "service" {
  source = "../../modules/ecs-service"

  name = "${var.name}-${var.environment}"

  vpc_id         = module.network.vpc_id
  alb_subnet_ids = module.network.public_subnet_ids
  # Tasks run in private subnets when NAT is enabled, else public with a public IP.
  task_subnet_ids  = var.enable_nat ? module.network.private_subnet_ids : module.network.public_subnet_ids
  assign_public_ip = !var.enable_nat

  image_tag           = var.image_tag
  desired_count       = var.desired_count
  acm_certificate_arn = var.acm_certificate_arn
  enable_otel         = var.enable_otel

  tags = local.tags
}
