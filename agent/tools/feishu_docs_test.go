package tools

import (
	"testing"

	"github.com/smallnest/goclaw/config"
)

func TestResolveFeishuCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cfg       config.FeishuChannelConfig
		wantAppID string
		wantToken string
	}{
		{
			name: "prefer top level credentials",
			cfg: config.FeishuChannelConfig{
				AppID:     "top-id",
				AppSecret: "top-secret",
				Accounts: map[string]config.ChannelAccountConfig{
					"acc1": {
						Enabled:   true,
						AppID:     "acc-id",
						AppSecret: "acc-secret",
					},
				},
			},
			wantAppID: "top-id",
			wantToken: "top-secret",
		},
		{
			name: "use enabled account when top level missing",
			cfg: config.FeishuChannelConfig{
				Accounts: map[string]config.ChannelAccountConfig{
					"acc1": {
						Enabled:   false,
						AppID:     "id-1",
						AppSecret: "secret-1",
					},
					"acc2": {
						Enabled:   true,
						AppID:     "id-2",
						AppSecret: "secret-2",
					},
				},
			},
			wantAppID: "id-2",
			wantToken: "secret-2",
		},
		{
			name: "fallback to sorted account when none enabled",
			cfg: config.FeishuChannelConfig{
				Accounts: map[string]config.ChannelAccountConfig{
					"b": {
						AppID:     "id-b",
						AppSecret: "secret-b",
					},
					"a": {
						AppID:     "id-a",
						AppSecret: "secret-a",
					},
				},
			},
			wantAppID: "id-a",
			wantToken: "secret-a",
		},
		{
			name: "return empty when no valid credentials",
			cfg: config.FeishuChannelConfig{
				AppID: "only-id",
				Accounts: map[string]config.ChannelAccountConfig{
					"acc1": {
						Enabled: true,
						AppID:   "",
					},
				},
			},
			wantAppID: "",
			wantToken: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotID, gotSecret := ResolveFeishuCredentials(tc.cfg)
			if gotID != tc.wantAppID || gotSecret != tc.wantToken {
				t.Fatalf("ResolveFeishuCredentials() = (%q, %q), want (%q, %q)",
					gotID, gotSecret, tc.wantAppID, tc.wantToken)
			}
		})
	}
}
