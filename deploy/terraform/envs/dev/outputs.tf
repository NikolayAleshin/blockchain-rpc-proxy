output "alb_dns_name" {
  description = "Public ALB DNS name — the proxy endpoint (http://<dns> or https:// with a cert)."
  value       = module.service.alb_dns_name
}

output "ecr_repository_url" {
  description = "ECR repository to push the image to before deploying."
  value       = module.service.ecr_repository_url
}

output "cluster_name" {
  description = "ECS cluster name."
  value       = module.service.cluster_name
}

output "service_name" {
  description = "ECS service name."
  value       = module.service.service_name
}
