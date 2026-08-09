// Package secrets detects values that look like unencrypted API keys or
// tokens (Stripe, AWS, OpenAI, GitHub, ...) so they can be flagged before
// being saved to a .env file or surfaced as potential leaks.
package secrets

import "regexp"

// Match is a single credential-like substring found in a value.
type Match struct {
	Name string // pattern name, e.g. "Stripe"
	Raw  string // the matched substring (treat as sensitive)
}

var rules = []struct {
	name string
	re   *regexp.Regexp
}{
	{"Stripe", regexp.MustCompile(`(sk|rk|pk)_(live|test)_[0-9a-zA-Z]{16,}`)},
	{"AWS Access Key", regexp.MustCompile(`(?:AKIA|ASIA|AIDA|AROA|AGPA)[0-9A-Z]{16}`)},
	{"AWS Session Token", regexp.MustCompile(`(?i)aws_session_token`)},
	{"OpenAI", regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`)},
	{"GitHub", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`)},
	{"GitHub PAT", regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`)},
	{"Slack", regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`)},
	{"Google API Key", regexp.MustCompile(`AIza[0-9A-Za-z\-_]{35}`)},
	{"Google OAuth Secret", regexp.MustCompile(`GOCSPX-[A-Za-z0-9_-]{20,}`)},
	{"SendGrid", regexp.MustCompile(`SG\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}`)},
	{"Twilio", regexp.MustCompile(`SK[0-9a-fA-F]{32}`)},
	{"npm", regexp.MustCompile(`npm_[A-Za-z0-9]{36}`)},
	{"Heroku", regexp.MustCompile(`heroku_[a-f0-9]{24}`)},
	{"JWT", regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`)},
	{"Private Key", regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |DSA |PGP )?PRIVATE KEY`)},
	{"Bearer Token", regexp.MustCompile(`(?i)bearer [A-Za-z0-9._~+/=-]{20,}`)},
}

// Detect returns every credential pattern matched inside value.
func Detect(value string) []Match {
	var out []Match
	for _, r := range rules {
		for _, loc := range r.re.FindAllStringIndex(value, -1) {
			out = append(out, Match{Name: r.name, Raw: value[loc[0]:loc[1]]})
		}
	}
	return out
}
