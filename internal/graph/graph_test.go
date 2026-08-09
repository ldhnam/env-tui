package graph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackend(t *testing.T) {
	cases := []struct {
		key, value, want string
	}{
		{"DATABASE_URL", "postgres://x", "PostgreSQL"},
		{"DB_URL", "mysql://x", "MySQL"},
		{"REDIS_URL", "redis://x", "Redis"},
		{"REDIS_CLUSTER_URL", "redis-cluster://x", "Redis"},
		{"STRIPE_SECRET_KEY", "sk_live_x", "Stripe"},
		{"STRIPE_WEBHOOK_KEY", "whsec_x", "Stripe"},
		{"JWT_SECRET", "abc", "Auth"},
		{"SENTRY_DSN", "https://x@sentry.io", "Sentry"},
		{"AWS_ACCESS_KEY_ID", "AKIAx", "AWS"},
		{"OPENAI_KEY", "sk-x", "OpenAI"},
		{"PORT", "3000", ""},
		{"NODE_ENV", "production", ""},
		{"DEBUG", "true", ""},
	}
	for _, c := range cases {
		if got := Backend(c.key, c.value); got != c.want {
			t.Errorf("Backend(%s, %q) = %q, want %q", c.key, c.value, got, c.want)
		}
	}
}

func TestRender(t *testing.T) {
	dir := t.TempDir()
	content := "DATABASE_URL=postgres://x\nREDIS_URL=redis://x\nPORT=3000\n"
	if err := os.WriteFile(filepath.Join(dir, ".env.local"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := Render(dir, "payment-api")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if !strings.Contains(out, "payment-api") {
		t.Errorf("missing service name:\n%s", out)
	}
	if !strings.Contains(lines[0], "┌") || !strings.Contains(lines[1], "payment-api") {
		t.Errorf("missing service box:\n%s", out)
	}
	if !strings.Contains(out, "DATABASE_URL") || !strings.Contains(out, "PostgreSQL") {
		t.Errorf("missing database node:\n%s", out)
	}
	if !strings.Contains(out, "REDIS_URL") || !strings.Contains(out, "Redis") {
		t.Errorf("missing redis node:\n%s", out)
	}
	// arrows and junctions present
	if !strings.Contains(out, "▼") || !strings.Contains(out, "┼") {
		t.Errorf("missing arrows/junction:\n%s", out)
	}
}
