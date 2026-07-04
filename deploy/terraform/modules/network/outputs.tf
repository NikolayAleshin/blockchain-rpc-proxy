output "vpc_id" {
  description = "VPC id."
  value       = aws_vpc.this.id
}

output "public_subnet_ids" {
  description = "Public subnet ids (ALB, and tasks when NAT is disabled)."
  value       = aws_subnet.public[*].id
}

output "private_subnet_ids" {
  description = "Private subnet ids (tasks when NAT is enabled)."
  value       = aws_subnet.private[*].id
}
