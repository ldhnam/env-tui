package secrets

import (
	"strings"
	"testing"
)

// join assembles test tokens from fragments so no full credential-shaped
// literal appears in the source (keeps GitHub push-protection happy while
// still exercising the matchers).
func join(parts ...string) string { return strings.Join(parts, "") }

func TestDetect(t *testing.T) {
	cases := []struct {
		value string
		names []string
	}{
		{join("sk_", "live_4eC39HqLyjWDarjtT1zdp7dc"), []string{"Stripe"}},
		{join("pk_test_", "51H3abcD4eF5gH6iJ7kL"), []string{"Stripe"}},
		{join("AKIA", "IOSFODNN7EXAMPLE"), []string{"AWS Access Key"}},
		{join("sk-proj-", "abcdEFGH1234xyz"), []string{"OpenAI"}},
		{join("ghp_", "1234567890abcdefghij"), []string{"GitHub"}},
		{join("github_pat_", "11AAbbCCddEEffGGhhII"), []string{"GitHub PAT"}},
		{join("xoxb-", "1234567890-abcdefgh"), []string{"Slack"}},
		{join("AIza", "SyA1234567890abcdefghijklmnopqrstuvwxyz"), []string{"Google API Key"}},
		{join("GOCSPX-", "abcdefghijklmnopqrstuvwxyz"), []string{"Google OAuth Secret"}},
		{join("SG.", "abcdefghijklmnopqrstuvwx.yzABCDEFGHIJKLMNOPQRST"), []string{"SendGrid"}},
		{join("SK", "0123456789abcdef0123456789abcdef"), []string{"Twilio"}},
		{join("npm_", "1234567890abcdefghijklmnopqrstuvwxyz"), []string{"npm"}},
		{join("heroku_", "0123456789abcdef01234567"), []string{"Heroku"}},
		{join("eyJ", "hbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjMifQ.abcdefghij"), []string{"JWT"}},
		{join("-----BEGIN RSA PRIVATE KEY-----", ""), []string{"Private Key"}},
		{join("Bearer eyJ", "hbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signedvalue12345"), []string{"JWT", "Bearer Token"}},
		// benign values must not match
		{"local-dev-key-123", nil},
		{"postgres://user:pass@host:5432/db", nil},
		{"redis://localhost:6379", nil},
		{"sk_live", nil},
		{"ghp_short", nil},
	}
	for _, c := range cases {
		got := Detect(c.value)
		if len(got) != len(c.names) {
			t.Errorf("Detect(%q) = %v, want %v", c.value, names(got), c.names)
			continue
		}
		for i, want := range c.names {
			if got[i].Name != want {
				t.Errorf("Detect(%q)[%d] = %q, want %q", c.value, i, got[i].Name, want)
			}
		}
	}
}

func names(ms []Match) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Name
	}
	return out
}
