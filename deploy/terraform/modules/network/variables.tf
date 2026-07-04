variable "name" {
  description = "Name prefix for network resources."
  type        = string
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC."
  type        = string
  default     = "10.0.0.0/16"
}

variable "azs" {
  description = "Availability zones to spread subnets across (>= 2 for HA)."
  type        = list(string)
}

variable "public_subnet_cidrs" {
  description = "CIDRs for the public subnets (one per AZ)."
  type        = list(string)
  default     = ["10.0.0.0/24", "10.0.1.0/24"]
}

variable "private_subnet_cidrs" {
  description = "CIDRs for the private subnets (one per AZ)."
  type        = list(string)
  default     = ["10.0.10.0/24", "10.0.11.0/24"]
}

variable "enable_nat" {
  description = "Create a NAT gateway so private-subnet tasks have egress. When false, tasks run in public subnets (cheaper for review; ~$32/mo NAT saved)."
  type        = bool
  default     = false
}

variable "enable_vpc_endpoints" {
  description = "Create interface endpoints (ECR api/dkr, CloudWatch Logs, SSM) + S3 gateway endpoint so tasks pull images/logs without NAT."
  type        = bool
  default     = false
}

variable "tags" {
  description = "Tags applied to all resources."
  type        = map(string)
  default     = {}
}
