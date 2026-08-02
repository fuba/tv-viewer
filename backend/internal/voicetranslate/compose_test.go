package voicetranslate

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTranslationComposeMountsTokenAtConfiguredPath(t *testing.T) {
	compose, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker is required to validate the Compose configuration")
	}
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate test source")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("test-token"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	command := exec.Command(compose, "compose", "-f", "docker-compose.yml", "-f", "compose.translation.yml", "config", "--format", "json")
	command.Dir = repositoryRoot
	command.Env = append(os.Environ(),
		"ALLOWED_ORIGINS=https://viewer.example.test",
		"VOICETRANSLATE_URL=wss://translate.example.test/ws",
		"VOICETRANSLATE_ORIGIN=https://translate.example.test",
		"VOICETRANSLATE_TOKEN_FILE="+tokenFile,
	)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("render Compose configuration: %v", err)
	}

	var config struct {
		Services map[string]struct {
			Environment map[string]string `json:"environment"`
			Secrets     []struct {
				Target string `json:"target"`
			} `json:"secrets"`
		} `json:"services"`
	}
	if err := json.Unmarshal(output, &config); err != nil {
		t.Fatalf("decode Compose configuration: %v", err)
	}
	backend := config.Services["backend"]
	want := backend.Environment["VOICETRANSLATE_TOKEN_FILE"]
	if len(backend.Secrets) != 1 {
		t.Fatalf("backend secrets = %d, want 1", len(backend.Secrets))
	}
	if got := backend.Secrets[0].Target; got != want {
		t.Fatalf("translation secret target = %q, want configured path %q", got, want)
	}
}
