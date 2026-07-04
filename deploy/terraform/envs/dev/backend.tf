# Remote state: S3 (encrypted, versioned) + DynamoDB lock.
# Config is supplied at init time so the bucket/table can differ per account:
#   tofu init \
#     -backend-config="bucket=<state-bucket>" \
#     -backend-config="key=polygon-rpc-proxy/dev/terraform.tfstate" \
#     -backend-config="region=<region>" \
#     -backend-config="dynamodb_table=<lock-table>" \
#     -backend-config="encrypt=true"
#
# For `tofu validate` use `tofu init -backend=false` (no remote state needed).
terraform {
  backend "s3" {}
}
