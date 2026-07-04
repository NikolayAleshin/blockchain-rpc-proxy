variable "name" {
  description = "Name prefix for the service resources."
  type        = string
}

variable "environment" {
  description = "Environment name (dev/staging/prod)."
  type        = string
}

variable "vpc_id" {
  description = "VPC id."
  type        = string
}

variable "alb_subnet_ids" {
  description = "Public subnet ids for the ALB."
  type        = list(string)
}

variable "task_subnet_ids" {
  description = "Subnet ids for the Fargate tasks (private when NAT enabled, public otherwise)."
  type        = list(string)
}

variable "assign_public_ip" {
  description = "Give tasks a public IP (required when they run in public subnets without NAT)."
  type        = bool
  default     = false
}

# --- image / container ---
variable "container_image" {
  description = "Full image URI to run. Defaults to the created ECR repo at :image_tag when empty."
  type        = string
  default     = ""
}

variable "image_tag" {
  description = "Image tag to deploy from the created ECR repo (used when container_image is empty)."
  type        = string
  default     = "latest"
}

variable "container_port" {
  description = "Container listen port."
  type        = number
  default     = 8080
}

variable "cpu" {
  description = "Fargate task CPU units."
  type        = number
  default     = 256
}

variable "memory" {
  description = "Fargate task memory (MiB)."
  type        = number
  default     = 512
}

# --- scaling ---
variable "desired_count" {
  description = "Initial task count."
  type        = number
  default     = 2
}

variable "min_capacity" {
  description = "Autoscaling minimum (>= 2 for multi-AZ HA)."
  type        = number
  default     = 2
}

variable "max_capacity" {
  description = "Autoscaling maximum."
  type        = number
  default     = 6
}

variable "cpu_target" {
  description = "Target average CPU utilization (%) for autoscaling."
  type        = number
  default     = 60
}

variable "requests_per_target" {
  description = "Target ALB requests per task for autoscaling."
  type        = number
  default     = 1000
}

# --- app config ---
variable "upstream_url" {
  description = "Upstream RPC endpoint."
  type        = string
  default     = "https://polygon.drpc.org"
}

variable "log_level" {
  description = "Application log level."
  type        = string
  default     = "info"
}

variable "metrics_enabled" {
  description = "Expose Prometheus /metrics."
  type        = bool
  default     = true
}

variable "extra_environment" {
  description = "Additional container environment variables."
  type        = map(string)
  default     = {}
}

# --- ingress / TLS ---
variable "acm_certificate_arn" {
  description = "ACM certificate ARN. When set, an HTTPS:443 listener is added and HTTP:80 redirects to it. When empty, only HTTP:80 is served."
  type        = string
  default     = ""
}

variable "health_check_path" {
  description = "ALB target-group health check path."
  type        = string
  default     = "/healthz"
}

variable "log_retention_days" {
  description = "CloudWatch log retention."
  type        = number
  default     = 14
}

variable "enable_otel" {
  description = "Add an ADOT collector sidecar and X-Ray permissions (target-state). Off by default to keep the review cheap."
  type        = bool
  default     = false
}

variable "tags" {
  description = "Tags applied to all resources."
  type        = map(string)
  default     = {}
}
