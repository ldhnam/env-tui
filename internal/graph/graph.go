// Package graph infers the backend/services a service's environment variables
// depend on and renders them as an ASCII dependency tree.
package graph

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

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

// Render draws a layered dependency diagram: the service in a box at the
// top, then each variable, then its inferred backend, aligned in columns.
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
	if len(keys) == 0 {
		return service + "\n", nil
	}

	n := len(keys)
	backends := make([]string, n)
	widths := make([]int, n)
	for i, k := range keys {
		backends[i] = Backend(k, env[k])
		widths[i] = max(lipgloss.Width(k), lipgloss.Width(backends[i]), 4)
	}
	starts := make([]int, n)
	x := 0
	for i := 0; i < n; i++ {
		starts[i] = x
		x += widths[i] + 3
	}
	full := x - 3 // width of the column band
	centers := make([]int, n)
	for i := 0; i < n; i++ {
		centers[i] = starts[i] + widths[i]/2
	}

	boxW := lipgloss.Width(service) + 4
	boxStart := max((full-boxW)/2, 0)
	boxEnd := boxStart + boxW - 1
	cx := boxStart + boxW/2
	bufLen := max(full, boxEnd) + 1

	mk := func() []rune {
		buf := make([]rune, bufLen)
		for i := range buf {
			buf[i] = ' '
		}
		return buf
	}
	place := func(buf []rune, center int, text string) {
		at := center - lipgloss.Width(text)/2
		if at < 0 {
			at = 0
		}
		for _, r := range text {
			if at < len(buf) {
				buf[at] = r
			}
			at++
		}
	}
	draw := func(buf []rune, indices []int, r rune) {
		for _, i := range indices {
			if i >= 0 && i < len(buf) {
				buf[i] = r
			}
		}
	}

	top := mk()
	draw(top, seq(boxStart+1, boxEnd), '─')
	draw(top, []int{boxStart}, '┌')
	draw(top, []int{boxEnd}, '┐')
	mid := mk()
	draw(mid, []int{boxStart, boxEnd}, '│')
	place(mid, cx, service)
	bot := mk()
	draw(bot, seq(boxStart+1, boxEnd), '─')
	draw(bot, []int{boxStart}, '└')
	draw(bot, []int{boxEnd}, '┘')
	draw(bot, []int{cx}, '┬')
	conn := mk()
	draw(conn, []int{cx}, '│')

	brd := mk()
	draw(brd, seq(1, full-1), '─')
	draw(brd, []int{0}, '┌')
	draw(brd, []int{full - 1}, '┐')
	draw(brd, []int{cx}, '┼')

	arr1 := mk()
	draw(arr1, centers, '▼')
	keyRow := mk()
	for i := 0; i < n; i++ {
		place(keyRow, centers[i], keys[i])
	}
	conn2 := mk()
	draw(conn2, centers, '│')
	arr2 := mk()
	draw(arr2, centers, '▼')
	beRow := mk()
	for i := 0; i < n; i++ {
		place(beRow, centers[i], backends[i])
	}

	var b strings.Builder
	for _, l := range [][]rune{top, mid, bot, conn, brd, arr1, keyRow, conn2, arr2, beRow} {
		b.WriteString(strings.TrimRight(string(l), " "))
		b.WriteString("\n")
	}
	return b.String(), nil
}

// seq returns [from, to).
func seq(from, to int) []int {
	var out []int
	for i := from; i < to; i++ {
		out = append(out, i)
	}
	return out
}

// Format renders a single dependency line, used by the TUI graph overlay.
func Format(service string, key, value string) string {
	backend := Backend(key, value)
	if backend == "" {
		return fmt.Sprintf("  %s", key)
	}
	return fmt.Sprintf("  %s ──▶ %s", key, backend)
}
