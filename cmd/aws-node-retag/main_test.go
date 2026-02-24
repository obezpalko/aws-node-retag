package main

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseInstanceID(t *testing.T) {
	cases := []struct {
		name       string
		providerID string
		want       string
		wantErr    bool
	}{
		{
			name:       "standard us-east-1",
			providerID: "aws:///us-east-1a/i-0abc123def456789a",
			want:       "i-0abc123def456789a",
		},
		{
			name:       "eu-west-1",
			providerID: "aws:///eu-west-1b/i-09876543210abcdef",
			want:       "i-09876543210abcdef",
		},
		{
			name:       "ap-southeast-2",
			providerID: "aws:///ap-southeast-2a/i-0abc123def456789a",
			want:       "i-0abc123def456789a",
		},
		{
			name:       "invalid - no i- prefix",
			providerID: "aws:///us-east-1a/invalid",
			wantErr:    true,
		},
		{
			name:       "empty string",
			providerID: "",
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseInstanceID(tc.providerID)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseInstanceID(%q) err=%v, wantErr=%v", tc.providerID, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("parseInstanceID(%q) = %q, want %q", tc.providerID, got, tc.want)
			}
		})
	}
}

func makePVWithAffinity(name string, terms []corev1.NodeSelectorTerm) *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.PersistentVolumeSpec{
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: terms,
				},
			},
		},
	}
}

func TestParseRegionFromPV(t *testing.T) {
	cases := []struct {
		name    string
		pv      *corev1.PersistentVolume
		want    string
		wantErr bool
	}{
		{
			name: "topology.kubernetes.io/region",
			pv: makePVWithAffinity("pv1", []corev1.NodeSelectorTerm{{
				MatchExpressions: []corev1.NodeSelectorRequirement{{
					Key:      "topology.kubernetes.io/region",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"us-east-1"},
				}},
			}}),
			want: "us-east-1",
		},
		{
			name: "topology.kubernetes.io/zone strips trailing char",
			pv: makePVWithAffinity("pv2", []corev1.NodeSelectorTerm{{
				MatchExpressions: []corev1.NodeSelectorRequirement{{
					Key:      "topology.kubernetes.io/zone",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"eu-west-1b"},
				}},
			}}),
			want: "eu-west-1",
		},
		{
			name: "topology.ebs.csi.aws.com/zone strips trailing char",
			pv: makePVWithAffinity("pv3", []corev1.NodeSelectorTerm{{
				MatchExpressions: []corev1.NodeSelectorRequirement{{
					Key:      "topology.ebs.csi.aws.com/zone",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"ap-southeast-2c"},
				}},
			}}),
			want: "ap-southeast-2",
		},
		{
			name: "no nodeAffinity returns error",
			pv: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{Name: "pv4"},
			},
			wantErr: true,
		},
		{
			name: "nodeAffinity with no matching key returns error",
			pv: makePVWithAffinity("pv5", []corev1.NodeSelectorTerm{{
				MatchExpressions: []corev1.NodeSelectorRequirement{{
					Key:      "some.other/label",
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{"value"},
				}},
			}}),
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRegionFromPV(tc.pv)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseRegionFromPV() err=%v, wantErr=%v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("parseRegionFromPV() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseRegion(t *testing.T) {
	cases := []struct {
		name       string
		providerID string
		want       string
		wantErr    bool
	}{
		{
			name:       "us-east-1a",
			providerID: "aws:///us-east-1a/i-0abc123def456789a",
			want:       "us-east-1",
		},
		{
			name:       "eu-west-1b",
			providerID: "aws:///eu-west-1b/i-09876543210abcdef",
			want:       "eu-west-1",
		},
		{
			name:       "ap-southeast-2c",
			providerID: "aws:///ap-southeast-2c/i-0abc123def456789a",
			want:       "ap-southeast-2",
		},
		{
			name:       "us-west-2a",
			providerID: "aws:///us-west-2a/i-0abc123def456789a",
			want:       "us-west-2",
		},
		{
			name:       "empty string",
			providerID: "",
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRegion(tc.providerID)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseRegion(%q) err=%v, wantErr=%v", tc.providerID, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("parseRegion(%q) = %q, want %q", tc.providerID, got, tc.want)
			}
		})
	}
}
