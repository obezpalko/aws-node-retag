# aws-node-retag

A lightweight Kubernetes controller that watches EKS node creation and PersistentVolume provisioning events, then applies AWS tags to the backing EC2 instances and EBS volumes. Useful for billing reports where managed nodepools do not allow custom tags at the nodegroup level.

## How it works

The controller runs two independent watchers:

**Node watcher** — fires when a node appears or its `ProviderID` is first set:
1. Parses the EC2 instance ID from `node.Spec.ProviderID`.
2. Calls `ec2:DescribeInstances` to find all attached EBS volumes.
3. Calls `ec2:CreateTags` on the instance and every attached volume.
4. Patches the node with annotation `aws-node-retag.io/tagged: "true"` to prevent re-tagging.

**PersistentVolume watcher** — fires when a PV transitions to `Bound` (dynamic provisioning):
1. Detects the EBS volume ID from the PV spec (CSI `ebs.csi.aws.com` or legacy `awsElasticBlockStore`).
2. Derives the AWS region from the PV's node affinity topology labels.
3. Calls `ec2:CreateTags` on the volume (retries up to 5× on `InvalidVolume.NotFound` — the CSI driver can mark a PV Bound before the volume is visible in the EC2 API).
4. Patches the PV with annotation `aws-node-retag.io/tagged: "true"` to prevent re-tagging.

Tags are configured once per cluster; all nodes and dynamically provisioned EBS volumes receive the same set of tags.

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

## Step 3 — Deploy with Helm

The image is published to GHCR — no build step required.

```bash
helm upgrade --install aws-node-retag \
  oci://ghcr.io/obezpalko/charts/aws-node-retag \
  --namespace kube-system \
  --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"="$ROLE_ARN" \
  --set tags.Environment=production \
  --set dryRun=false
```

To apply multiple tags:

```bash
helm upgrade --install aws-node-retag \
  oci://ghcr.io/obezpalko/charts/aws-node-retag \
  --namespace kube-system \
  --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"="$ROLE_ARN" \
  --set tags.Environment=production \
  --set tags.Team=platform \
  --set tags.CostCenter=eng-123 \
  --set dryRun=false
```

Or with a values file:

```yaml
# values-prod.yaml
serviceAccount:
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/AWSNodeRetagRole

tags:
  Environment: production
  Team: platform
  CostCenter: eng-123

dryRun: false
```

```bash
helm upgrade --install aws-node-retag \
  oci://ghcr.io/obezpalko/charts/aws-node-retag \
  --namespace kube-system \
  -f values-prod.yaml
```

---

## Step 4 — Verify

```bash
# Check the pod is running
kubectl -n kube-system get pods -l app.kubernetes.io/name=aws-node-retag

# Stream logs
kubectl -n kube-system logs -l app.kubernetes.io/name=aws-node-retag -f

# After a node joins, check the annotation
kubectl get node <NODE-NAME> \
  -o jsonpath='{.metadata.annotations.aws-node-retag\.io/tagged}'
# Expected: true

# After a PVC is created, check the PV annotation
kubectl get pv <PV-NAME> \
  -o jsonpath='{.metadata.annotations.aws-node-retag\.io/tagged}'
# Expected: true

# Confirm the tags in AWS
aws ec2 describe-tags \
  --filters "Name=resource-id,Values=<INSTANCE-OR-VOLUME-ID>" \
            "Name=key,Values=Environment"
```

---

## Configuration reference

| Helm value | Default | Description |
|---|---|---|
| `image.repository` | `ghcr.io/obezpalko/aws-node-retag` | Container image repository |
| `image.tag` | Chart `appVersion` | Image tag |
| `serviceAccount.annotations` | `{}` | Use to set the IRSA role ARN (`eks.amazonaws.com/role-arn`) |
| `tags` | `{}` *(required, min 1 entry)* | Map of AWS tags to apply to instances and volumes |
| `dryRun` | `true` | Log what would be tagged without making any AWS or Kubernetes writes |
| `namespace` | `kube-system` | Kubernetes namespace |
| `serviceAccount.name` | `aws-node-retag` | ServiceAccount name |
| `replicaCount` | `1` | Keep at 1 to avoid annotation races |
| `nodeSelector` | `{}` | Schedule on a specific nodepool |
| `priorityClassName` | `""` | Pod priority class (e.g. `system-cluster-critical`) |
| `resources.requests` | `50m / 64Mi` | CPU and memory requests |
| `resources.limits` | `200m / 128Mi` | CPU and memory limits |

## Development

```bash
# Run unit tests (no AWS or Kubernetes required)
go test ./...

# Lint
go vet ./...
```
