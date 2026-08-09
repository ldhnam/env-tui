// Package vault integrates with secret managers (Doppler, HashiCorp Vault,
// 1Password CLI, AWS Secrets Manager, Infisical) via their CLIs. It provides
// read-only fetch and pull/push of environment key/value pairs.
package vault

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// Provider identifies a secret manager CLI.
type Provider string

const (
	Doppler     Provider = "doppler"
	HashiVault  Provider = "vault"
	OnePassword Provider = "op"
	AWS         Provider = "aws"
	Infisical   Provider = "infisical"
)

// Supported reports whether p is a known provider.
func Supported(p Provider) bool {
	switch p {
	case Doppler, HashiVault, OnePassword, AWS, Infisical:
		return true
	}
	return false
}

// Fetch pulls all secrets for the provider's scope.
func Fetch(p Provider, project, env string) (map[string]string, error) {
	switch p {
	case Doppler:
		return dopplerFetch(project, env)
	case HashiVault:
		return vaultFetch(project)
	case OnePassword:
		return opFetch(project, env)
	case AWS:
		return awsFetch(project, env)
	case Infisical:
		return infisicalFetch(project, env)
	}
	return nil, fmt.Errorf("unknown vault provider %q", p)
}

// Push writes secrets to the provider's scope.
func Push(p Provider, project, env string, secrets map[string]string) error {
	switch p {
	case Doppler:
		return dopplerPush(project, env, secrets)
	case HashiVault:
		return vaultPush(project, secrets)
	case OnePassword:
		return opPush(project, secrets)
	case AWS:
		return awsPush(project, env, secrets)
	case Infisical:
		return infisicalPush(project, env, secrets)
	}
	return fmt.Errorf("unknown vault provider %q", p)
}

// --- Doppler ---

func dopplerFetch(project, env string) (map[string]string, error) {
	args := []string{"secrets", "download", "--no-file", "--format=json"}
	if project != "" {
		args = append(args, "--project", project)
	}
	if env != "" {
		args = append(args, "--config", env)
	}
	out, err := runCommand("doppler", args...)
	if err != nil {
		return nil, err
	}
	return parseJSONSecretMap(out)
}

func dopplerPush(project, env string, secrets map[string]string) error {
	args := []string{"secrets", "set"}
	if project != "" {
		args = append(args, "--project", project)
	}
	if env != "" {
		args = append(args, "--config", env)
	}
	for _, k := range sortedKeys(secrets) {
		args = append(args, k+"="+secrets[k])
	}
	_, err := runCommand("doppler", args...)
	return err
}

// --- HashiCorp Vault (KV v1 and v2) ---

func vaultFetch(path string) (map[string]string, error) {
	out, err := runCommand("vault", "kv", "get", "-format=json", path)
	if err != nil {
		return nil, err
	}
	return parseVaultData(out)
}

func vaultPush(path string, secrets map[string]string) error {
	args := []string{"kv", "put", path}
	for _, k := range sortedKeys(secrets) {
		args = append(args, k+"="+secrets[k])
	}
	_, err := runCommand("vault", args...)
	return err
}

// parseVaultData handles both `{"data":{...}}` (KV v1) and
// `{"data":{"data":{...},"metadata":{...}}}` (KV v2).
func parseVaultData(out []byte) (map[string]string, error) {
	var top struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(out, &top); err != nil {
		return nil, err
	}
	secrets := make(map[string]string)
	if inner, ok := top.Data["data"].(map[string]interface{}); ok {
		for k, v := range inner {
			secrets[k] = stringify(v)
		}
		return secrets, nil
	}
	for k, v := range top.Data {
		secrets[k] = stringify(v)
	}
	return secrets, nil
}

// --- 1Password CLI (op) ---

func opFetch(item, vaultName string) (map[string]string, error) {
	args := []string{"item", "get", item, "--format=json"}
	if vaultName != "" {
		args = append(args, "--vault", vaultName)
	}
	out, err := runCommand("op", args...)
	if err != nil {
		return nil, err
	}
	var fields []struct {
		Label string `json:"label"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(out, &fields); err != nil {
		return nil, err
	}
	secrets := make(map[string]string)
	for _, f := range fields {
		if f.Label != "" && f.Value != "" {
			secrets[f.Label] = f.Value
		}
	}
	return secrets, nil
}

func opPush(item string, secrets map[string]string) error {
	args := []string{"item", "edit", item}
	for _, k := range sortedKeys(secrets) {
		args = append(args, k+"="+secrets[k])
	}
	_, err := runCommand("op", args...)
	return err
}

// --- AWS Secrets Manager ---

func awsFetch(region string, _ string) (map[string]string, error) {
	args := []string{"secretsmanager", "list-secrets", "--query", "SecretList[].Name", "--output", "json"}
	if region != "" {
		args = append(args, "--region", region)
	}
	out, err := runCommand("aws", args...)
	if err != nil {
		return nil, err
	}
	var names []string
	if err := json.Unmarshal(out, &names); err != nil {
		return nil, err
	}
	secrets := make(map[string]string)
	for _, name := range names {
		val, gerr := awsGetSecret(name, region)
		if gerr != nil {
			continue
		}
		var obj map[string]string
		if json.Unmarshal([]byte(val), &obj) == nil && obj != nil {
			for k, v := range obj {
				secrets[k] = v
			}
		} else {
			secrets[name] = val
		}
	}
	return secrets, nil
}

func awsGetSecret(name, region string) (string, error) {
	args := []string{"secretsmanager", "get-secret-value", "--secret-id", name, "--query", "SecretString", "--output", "text"}
	if region != "" {
		args = append(args, "--region", region)
	}
	out, err := runCommand("aws", args...)
	return strings.TrimSpace(string(out)), err
}

func awsPush(region string, _ string, secrets map[string]string) error {
	keys := sortedKeys(secrets)
	for _, k := range keys {
		args := []string{"secretsmanager", "put-secret-value", "--secret-id", k, "--secret-string", secrets[k]}
		if region != "" {
			args = append(args, "--region", region)
		}
		if _, err := runCommand("aws", args...); err != nil {
			return err
		}
	}
	return nil
}

// --- Infisical ---

func infisicalFetch(_ string, env string) (map[string]string, error) {
	args := []string{"secrets"}
	if env != "" {
		args = append(args, "--env", env)
	}
	args = append(args, "--plain")
	out, err := runCommand("infisical", args...)
	if err != nil {
		return nil, err
	}
	return parseKeyValueLines(out)
}

func infisicalPush(_ string, env string, secrets map[string]string) error {
	args := []string{"secrets", "set"}
	for _, k := range sortedKeys(secrets) {
		args = append(args, k+"="+secrets[k])
	}
	if env != "" {
		args = append(args, "--env", env)
	}
	_, err := runCommand("infisical", args...)
	return err
}

// --- helpers ---

func runCommand(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return buf.Bytes(), fmt.Errorf("%s: %v: %s", name, err, strings.TrimSpace(buf.String()))
	}
	return buf.Bytes(), nil
}

// parseJSONSecretMap accepts both flat `{"K":"v"}` and Doppler's nested
// `{"K":{"raw":"v"}}` formats.
func parseJSONSecretMap(out []byte) (map[string]string, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}
	secrets := make(map[string]string, len(raw))
	for k, v := range raw {
		var s string
		if json.Unmarshal(v, &s) == nil {
			secrets[k] = s
			continue
		}
		var obj struct {
			Raw string `json:"raw"`
		}
		if json.Unmarshal(v, &obj) == nil && obj.Raw != "" {
			secrets[k] = obj.Raw
		}
	}
	return secrets, nil
}

// parseKeyValueLines parses `KEY=VALUE` output (Infisical --plain).
func parseKeyValueLines(out []byte) (map[string]string, error) {
	secrets := make(map[string]string)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "="); i > 0 {
			secrets[strings.TrimSpace(line[:i])] = strings.TrimSpace(line[i+1:])
		}
	}
	return secrets, nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func stringify(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case nil:
		return ""
	default:
		return fmt.Sprint(s)
	}
}
