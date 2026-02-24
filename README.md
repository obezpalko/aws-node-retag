# aws-node-retag

A lightweight Kubernetes controller that watches for EKS node creation events and applies additional AWS tags to the EC2 instance and its EBS volumes. Useful for billing reports where managed nodepools do not allow custom tags at the nodegroup level.

## How it works

1. Runs as a single-replica Deployment on the `system` nodepool.
2. Watches the Kubernetes Node API via a SharedInformer.
3. When a node appears (or when its `ProviderID` is first set), the controller:
   - Parses the EC2 instance ID from `node.Spec.ProviderID`.
   - Calls `ec2:DescribeInstances` to find attached EBS volumes.
   - Calls `ec2:CreateTags` on the instance and all volumes.
   - Patches the node with annotation `aws-node-retag.io/tagged: "true"` to prevent re-tagging.

Tags are configured once per cluster; all nodes in every nodepool receive the same tags.

## Prerequisites

- EKS cluster with OIDC provider enabled.
- `aws` CLI, `kubectl`, and `helm` v3.

---

## Step 1 — Create the IAM Policy

```bash
aws iam create-policy \
  --policy-name AWSNodeRetagPolicy \
  --policy-document file://iam/policy.json
```

Note the returned `Arn` — you will need it in Step 2.

---

## Step 2 — Create the IAM Role (IRSA)

### 2a. Collect cluster details

```bash
CLUSTER_NAME=<your-cluster-name>
REGION=$(aws eks describe-cluster --name "$CLUSTER_NAME" \
  --query "cluster.resourcesVpcConfig.vpcId" --output text | xargs -I{} \
  aws ec2 describe-vpcs --vpc-ids {} --query "Vpcs[0].OwnerId" --output text)

# Simpler: get region from your kubeconfig context or AWS CLI config
REGION=<your-region>
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)

OIDC_ID=$(aws eks describe-cluster \
  --name "$CLUSTER_NAME" \
  --query "cluster.identity.oidc.issuer" \
  --output text | awk -F'/' '{print $NF}')

echo "Account: $ACCOUNT_ID  Region: $REGION  OIDC ID: $OIDC_ID"
```

### 2b. Populate the trust policy

```bash
sed \
  -e "s/ACCOUNT_ID/$ACCOUNT_ID/g" \
  -e "s/REGION/$REGION/g"         \
  -e "s/OIDC_ID/$OIDC_ID/g"       \
  iam/trust-policy.json > /tmp/trust-policy.json
```

### 2c. Create the role and attach the policy

```bash
POLICY_ARN=arn:aws:iam::${ACCOUNT_ID}:policy/AWSNodeRetagPolicy

aws iam create-role \
  --role-name AWSNodeRetagRole \
  --assume-role-policy-document file:///tmp/trust-policy.json

aws iam attach-role-policy \
  --role-name AWSNodeRetagRole \
  --policy-arn "$POLICY_ARN"

ROLE_ARN=$(aws iam get-role --role-name AWSNodeRetagRole \
  --query Role.Arn --output text)
echo "Role ARN: $ROLE_ARN"
```

---

## Step 3 — Build and push the Docker image

```bash
IMAGE=<your-registry>/aws-node-retag:0.1.0

# Resolve Go dependencies first
go mod tidy

docker build -t "$IMAGE" .
docker push "$IMAGE"
```

---

## Step 4 — Deploy with Helm

```bash
helm upgrade --install aws-node-retag helm/aws-node-retag \
  --namespace kube-system \
  --set image.repository=<your-registry>/aws-node-retag \
  --set image.tag=0.1.0 \
  --set irsaRoleArn="$ROLE_ARN" \
  --set tags.Environment=production
```

To add multiple tags:

```bash
helm upgrade --install aws-node-retag helm/aws-node-retag \
  --namespace kube-system \
  --set irsaRoleArn="$ROLE_ARN" \
  --set tags.Environment=production \
  --set tags.Team=platform \
  --set tags.CostCenter=eng-123
```

---

## Step 5 — Verify

```bash
# Check the pod is running
kubectl -n kube-system get pods -l app.kubernetes.io/name=aws-node-retag

# Stream logs
kubectl -n kube-system logs -l app.kubernetes.io/name=aws-node-retag -f

# After a node joins, check the annotation
kubectl get node <NODE-NAME> \
  -o jsonpath='{.metadata.annotations.aws-node-retag\.io/tagged}'
# Expected: true

# Confirm the tag in AWS
aws ec2 describe-tags \
  --filters "Name=resource-id,Values=<INSTANCE-ID>" \
            "Name=key,Values=Environment"
```

---

## Configuration reference

| Helm value | Default | Description |
|---|---|---|
| `image.repository` | `ghcr.io/obezpalko/aws-node-retag` | Container image repository |
| `image.tag` | Chart `appVersion` | Image tag |
| `irsaRoleArn` | *(required)* | ARN of the IAM role |
| `tags` | `{}` *(required)* | Map of AWS tags to apply |
| `namespace` | `kube-system` | Kubernetes namespace |
| `serviceAccount.name` | `aws-node-retag` | ServiceAccount name |
| `nodeSelector` | `eks.amazonaws.com/nodegroup: system` | Schedule on system nodepool |
| `replicaCount` | `1` | Keep at 1 to avoid annotation races |

## Development

```bash
# Run unit tests (no AWS or Kubernetes required)
go test ./...

# Lint
go vet ./...
```
