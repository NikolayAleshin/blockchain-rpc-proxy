# Terraform / OpenTofu — AWS ECS Fargate

Provisions the proxy on **ECS Fargate** behind an **Application Load Balancer**,
with ECR, CloudWatch Logs, IAM, autoscaling and zero-downtime deploys.

> Examples use `tofu` (OpenTofu). Swap for `terraform` — the config is compatible.

## Layout

```
modules/
  network/       VPC, 2-AZ public/private subnets, IGW, optional NAT + VPC endpoints
  ecs-service/   ECR, logs, IAM, SGs, ALB+TG+listeners, task def, service, autoscaling
envs/
  dev/           root module wiring network + service for the dev environment
```

## Prerequisites

- AWS credentials (`aws configure`, SSO, or CI via GitHub OIDC).
- An S3 bucket + DynamoDB table for remote state (see `envs/dev/backend.tf`).

## Validate (no cloud needed)

```bash
cd envs/dev
tofu init -backend=false
tofu validate
tofu fmt -check -recursive
```

## Deploy

```bash
cd envs/dev
cp terraform.tfvars.example terraform.tfvars   # edit as needed

tofu init \
  -backend-config="bucket=<state-bucket>" \
  -backend-config="key=polygon-rpc-proxy/dev/terraform.tfstate" \
  -backend-config="region=<region>" \
  -backend-config="dynamodb_table=<lock-table>" \
  -backend-config="encrypt=true"

tofu apply
```

Then build & push the image to the created ECR repo and roll the service:

```bash
ECR=$(tofu output -raw ecr_repository_url)
aws ecr get-login-password | docker login --username AWS --password-stdin "${ECR%/*}"
docker build -t "$ECR:$(git rev-parse --short HEAD)" ../../..
docker push "$ECR:$(git rev-parse --short HEAD)"
tofu apply -var="image_tag=$(git rev-parse --short HEAD)"

curl -s -X POST "http://$(tofu output -raw alb_dns_name)" \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber"}'
```

## Cost & teardown

- Default `enable_nat=false` runs tasks in public subnets (**no NAT gateway**, saving
  ~$32/mo) — good for cheap, non-production environments. Set `enable_nat=true` for
  the private production posture.
- **Always tear down when done:**

```bash
tofu destroy
```

## Toggles (see `variables.tf`)

| Var | Default | Effect |
|-----|---------|--------|
| `enable_nat` | `false` | Private subnets + NAT (prod) vs public subnets (cheap) |
| `enable_vpc_endpoints` | `false` | ECR/Logs/SSM/S3 endpoints (pull without NAT) |
| `enable_otel` | `false` | ADOT collector sidecar + X-Ray permissions |
| `acm_certificate_arn` | `""` | Set to serve HTTPS (and redirect HTTP→HTTPS) |
| `desired_count` | `2` | Initial tasks (autoscales `min_capacity`..`max_capacity`) |

## Notable design

- **Zero-downtime**: rolling update, `min 100% / max 200%`, **deployment circuit
  breaker with auto-rollback**, ALB health-check gating + connection draining.
- **Autoscaling**: target tracking on CPU and `ALBRequestCountPerTarget`, min ≥ 2 across 2 AZs.
- **Security**: ALB→task on 8080 only, tasks private (with NAT) or public-IP (without),
  non-root distroless image, ECR scan-on-push, consistent cost-allocation tags.
