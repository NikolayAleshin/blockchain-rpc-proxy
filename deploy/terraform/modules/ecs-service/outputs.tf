output "alb_dns_name" {
  description = "Public DNS name of the ALB (point clients / Cloudflare here)."
  value       = aws_lb.this.dns_name
}

output "ecr_repository_url" {
  description = "ECR repository URL to push the image to."
  value       = aws_ecr_repository.this.repository_url
}

output "cluster_name" {
  description = "ECS cluster name."
  value       = aws_ecs_cluster.this.name
}

output "service_name" {
  description = "ECS service name."
  value       = aws_ecs_service.this.name
}

output "log_group" {
  description = "CloudWatch log group."
  value       = aws_cloudwatch_log_group.this.name
}
