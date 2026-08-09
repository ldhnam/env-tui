// Package graph infers the backend/services a service's environment variables
// depend on and renders them as an ASCII dependency tree.
package graph

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ldhnam/envigator/internal/envfile"
)

var backends = []struct{ name, needle string }{
	{"PostgreSQL", "postgresql"},
	{"PostgreSQL", "postgres"},
	{"MySQL", "mysql"},
	{"MongoDB", "mongodb"},
	{"MongoDB", "mongo"},
	{"SQLite", "sqlite"},
	{"Redis", "redis"},
	{"RabbitMQ", "rabbitmq"},
	{"RabbitMQ", "amqp"},
	{"Kafka", "kafka"},
	{"Elasticsearch", "elasticsearch"},
	{"Elasticsearch", "elastic"},
	{"OpenAI", "openai"},
	{"Stripe", "stripe"},
	{"Sentry", "sentry"},
	{"Slack", "slack"},
	{"Discord", "discord"},
	{"Twilio", "twilio"},
	{"GitHub", "github"},
	{"Google", "gcp"},
	{"Google", "google"},
	{"AWS", "aws"},
	{"S3", "s3"},
	{"Auth", "oauth"},
	{"Auth", "jwt"},
	{"Auth", "session"},
	{"Auth", "auth"},
}

// Backend infers the technology or service a variable relates to from its key
// name and value, or "" when nothing obvious matches.
func Backend(key, value string) string {
	pair := strings.ToLower(key + " " + value)
	for _, b := range backends {
		if strings.Contains(pair, b.needle) {
			return b.name
		}
	}
	return ""
}

// Render draws the dependency tree for a service and its variables.
func Render(dir, service string) (string, error) {
	env, err := envfile.LoadForRun(dir)
	if err != nil {
		return "", err
	}
	if service == "" {
		service = filepath.Base(dir)
		if service == "." || service == "/" || service == "" {
			service = "service"
		}
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(service)
	b.WriteString("\n│\n")
	for i, k := range keys {
		last := i == len(keys)-1
		conn := "├── "
		childConn := "│   "
		if last {
			conn = "└── "
			childConn = "    "
		}
		b.WriteString(conn + k + "\n")
		if backend := Backend(k, env[k]); backend != "" {
			b.WriteString(childConn + "└── " + backend + "\n")
		}
		if !last {
			b.WriteString("│\n")
		}
	}
	return b.String(), nil
}

// Format renders a single dependency line, used by the TUI graph overlay.
func Format(service string, key, value string) string {
	backend := Backend(key, value)
	if backend == "" {
		return fmt.Sprintf("  %s", key)
	}
	return fmt.Sprintf("  %s ──▶ %s", key, backend)
}
