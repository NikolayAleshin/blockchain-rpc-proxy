variable "aws_region" {
  description = "AWS region to deploy into."
  type        = string
  default     = "eu-central-1"
}

variable "name" {
  description = "Base name for all resources."
  type        = string
  default     = "polygon-rpc-proxy"
}

variable "environment" {
  description = "Environment name."
  type        = string
  default     = "dev"
}

variable "owner" {
  description = "Owner tag (cost allocation / governance)."
  type        = string
  default     = "platform"
}

variable "cost_center" {
  description = "Cost-center tag."
  type        = string
  default     = "engineering"
}

variable "image_tag" {
  description = "Image tag to deploy from ECR (use the git SHA in CI)."
  type        = string
  default     = "latest"
}

variable "enable_nat" {
  description = "Run tasks in private subnets behind a NAT gateway (prod posture). False = public subnets, cheaper for review."
  type        = bool
  default     = false
}

variable "enable_vpc_endpoints" {
  description = "Create VPC endpoints for ECR/Logs/SSM/S3 (image pull + logs without NAT)."
  type        = bool
  default     = false
}

variable "enable_otel" {
  description = "Add the ADOT collector sidecar + X-Ray permissions (target-state)."
  type        = bool
  default     = false
}

variable "acm_certificate_arn" {
  description = "ACM cert ARN for HTTPS. Empty = HTTP only (fine for a demo without a domain)."
  type        = string
  default     = ""
}

variable "desired_count" {
  description = "Initial task count."
  type        = number
  default     = 2
}
