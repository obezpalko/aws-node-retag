package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const (
	annotationKey   = "aws-node-retag.io/tagged"
	annotationValue = "true"
	resyncPeriod    = 12 * time.Hour
)

type Tagger struct {
	k8s    kubernetes.Interface
	ec2    *ec2.Client
	tags   map[string]string
	logger *slog.Logger
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	tagsRaw := os.Getenv("TAGS")
	if tagsRaw == "" {
		logger.Error(`TAGS environment variable is required (JSON object, e.g. {"Environment":"production"})`)
		os.Exit(1)
	}
	var tags map[string]string
	if err := json.Unmarshal([]byte(tagsRaw), &tags); err != nil {
		logger.Error("failed to parse TAGS", "error", err, "value", tagsRaw)
		os.Exit(1)
	}
	if len(tags) == 0 {
		logger.Error("TAGS must contain at least one key-value pair")
		os.Exit(1)
	}
	logger.Info("loaded tags", "tags", tags)

	k8sCfg, err := rest.InClusterConfig()
	if err != nil {
		logger.Error("failed to build in-cluster k8s config", "error", err)
		os.Exit(1)
	}
	k8sClient, err := kubernetes.NewForConfig(k8sCfg)
	if err != nil {
		logger.Error("failed to create k8s client", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		logger.Error("failed to load AWS config", "error", err)
		os.Exit(1)
	}
	ec2Client := ec2.NewFromConfig(awsCfg)

	tagger := &Tagger{
		k8s:    k8sClient,
		ec2:    ec2Client,
		tags:   tags,
		logger: logger,
	}

	factory := informers.NewSharedInformerFactory(k8sClient, resyncPeriod)
	nodeInformer := factory.Core().V1().Nodes().Informer()

	nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node, ok := obj.(*corev1.Node)
			if !ok {
				return
			}
			tagger.handleNode(ctx, node)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldNode, ok1 := oldObj.(*corev1.Node)
			newNode, ok2 := newObj.(*corev1.Node)
			if !ok1 || !ok2 {
				return
			}
			// Only act when ProviderID transitions from empty to set.
			// This handles the case where cloud-controller-manager sets the
			// ProviderID after the node first appears in the API.
			if oldNode.Spec.ProviderID == "" && newNode.Spec.ProviderID != "" {
				tagger.handleNode(ctx, newNode)
			}
		},
	})

	stopCh := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	factory.Start(stopCh)
	logger.Info("waiting for cache sync")
	if !cache.WaitForCacheSync(stopCh, nodeInformer.HasSynced) {
		logger.Error("timed out waiting for cache sync")
		close(stopCh)
		os.Exit(1)
	}
	logger.Info("cache synced, watching for nodes")

	<-sigCh
	logger.Info("shutting down")
	close(stopCh)
}

// handleNode tags the EC2 instance and its EBS volumes for a given node.
// It is idempotent: nodes that already carry the tagged annotation are skipped.
func (t *Tagger) handleNode(ctx context.Context, node *corev1.Node) {
	log := t.logger.With("node", node.Name)

	if node.Annotations[annotationKey] == annotationValue {
		log.Debug("node already tagged, skipping")
		return
	}

	if node.Spec.ProviderID == "" {
		log.Info("providerID not yet set, will retry on UpdateFunc")
		return
	}

	if !strings.HasPrefix(node.Spec.ProviderID, "aws://") {
		log.Warn("not an AWS node, skipping", "providerID", node.Spec.ProviderID)
		return
	}

	instanceID, err := parseInstanceID(node.Spec.ProviderID)
	if err != nil {
		log.Error("failed to parse instance ID", "providerID", node.Spec.ProviderID, "error", err)
		return
	}

	region, err := parseRegion(node.Spec.ProviderID)
	if err != nil {
		log.Error("failed to parse region", "providerID", node.Spec.ProviderID, "error", err)
		return
	}

	log = log.With("instanceID", instanceID, "region", region)
	log.Info("tagging node")

	volumeIDs, err := t.listAttachedVolumes(ctx, region, instanceID)
	if err != nil {
		log.Error("failed to list attached volumes", "error", err)
		return
	}

	resources := append([]string{instanceID}, volumeIDs...)

	if err := t.applyTags(ctx, region, resources); err != nil {
		log.Error("failed to apply tags", "error", err)
		return
	}

	if err := t.annotateNode(ctx, node.Name); err != nil {
		log.Error("failed to annotate node (tags were applied)", "error", err)
		return
	}

	log.Info("node tagged successfully", "volumes", len(volumeIDs))
}

// parseInstanceID extracts the EC2 instance ID from a node ProviderID.
// Expected format: aws:///us-east-1a/i-0123456789abcdef0
func parseInstanceID(providerID string) (string, error) {
	parts := strings.Split(providerID, "/")
	id := parts[len(parts)-1]
	if !strings.HasPrefix(id, "i-") {
		return "", fmt.Errorf("expected instance ID starting with 'i-', got %q (providerID: %s)", id, providerID)
	}
	return id, nil
}

// parseRegion derives the AWS region from a node ProviderID.
// Expected format: aws:///us-east-1a/i-xxx → strips the trailing AZ letter.
func parseRegion(providerID string) (string, error) {
	parts := strings.Split(providerID, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("unexpected providerID format: %s", providerID)
	}
	az := parts[len(parts)-2]
	if len(az) < 2 {
		return "", fmt.Errorf("AZ too short to derive region: %q (providerID: %s)", az, providerID)
	}
	return az[:len(az)-1], nil
}

// listAttachedVolumes returns the EBS volume IDs attached to the given instance.
func (t *Tagger) listAttachedVolumes(ctx context.Context, region, instanceID string) ([]string, error) {
	out, err := t.ec2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	}, func(o *ec2.Options) {
		o.Region = region
	})
	if err != nil {
		return nil, fmt.Errorf("DescribeInstances: %w", err)
	}

	var volumeIDs []string
	for _, r := range out.Reservations {
		for _, inst := range r.Instances {
			for _, bdm := range inst.BlockDeviceMappings {
				if bdm.Ebs != nil && bdm.Ebs.VolumeId != nil {
					volumeIDs = append(volumeIDs, *bdm.Ebs.VolumeId)
				}
			}
		}
	}
	return volumeIDs, nil
}

// applyTags calls ec2:CreateTags on the given resource IDs (instance + volumes).
func (t *Tagger) applyTags(ctx context.Context, region string, resourceIDs []string) error {
	ec2Tags := make([]ec2types.Tag, 0, len(t.tags))
	for k, v := range t.tags {
		ec2Tags = append(ec2Tags, ec2types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	_, err := t.ec2.CreateTags(ctx, &ec2.CreateTagsInput{
		Resources: resourceIDs,
		Tags:      ec2Tags,
	}, func(o *ec2.Options) {
		o.Region = region
	})
	if err != nil {
		return fmt.Errorf("CreateTags: %w", err)
	}
	return nil
}

// annotateNode patches the node with the idempotency annotation.
func (t *Tagger) annotateNode(ctx context.Context, nodeName string) error {
	patch := fmt.Sprintf(`{"metadata":{"annotations":{%q:%q}}}`, annotationKey, annotationValue)
	_, err := t.k8s.CoreV1().Nodes().Patch(
		ctx,
		nodeName,
		types.MergePatchType,
		[]byte(patch),
		metav1.PatchOptions{},
	)
	return err
}
