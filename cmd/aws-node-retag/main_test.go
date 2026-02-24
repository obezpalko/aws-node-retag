package main

import "testing"

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
