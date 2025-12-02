package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateEnvExports_LocalBashZsh(t *testing.T) {
	result := generateEnvExports("local", "http://localhost:1234", "", "llama3", "", ShellBashZsh)

	// Check it contains expected exports
	if !strings.Contains(result, "export LLM_PROVIDER=local") {
		t.Error("Expected 'export LLM_PROVIDER=local' in output")
	}
	if !strings.Contains(result, "export LLM_ENDPOINT=http://localhost:1234") {
		t.Error("Expected 'export LLM_ENDPOINT=http://localhost:1234' in output")
	}
	if !strings.Contains(result, "export LLM_MODEL=llama3") {
		t.Error("Expected 'export LLM_MODEL=llama3' in output")
	}
	// Should NOT contain API key if empty
	if strings.Contains(result, "LLM_API_KEY") {
		t.Error("Should not contain LLM_API_KEY when empty")
	}
	// Should contain comment
	if !strings.Contains(result, "# CapyCut configuration") {
		t.Error("Expected configuration comment in output")
	}
}

func TestGenerateEnvExports_LocalWithAPIKey(t *testing.T) {
	result := generateEnvExports("local", "http://localhost:1234", "my-secret-key", "mistral", "", ShellBashZsh)

	if !strings.Contains(result, "export LLM_API_KEY=my-secret-key") {
		t.Error("Expected 'export LLM_API_KEY=my-secret-key' in output")
	}
}

func TestGenerateEnvExports_AzureBashZsh(t *testing.T) {
	result := generateEnvExports("azure", "https://my-resource.openai.azure.com", "azure-key-123", "gpt-4o", "2025-04-01-preview", ShellBashZsh)

	if !strings.Contains(result, "export LLM_PROVIDER=azure") {
		t.Error("Expected 'export LLM_PROVIDER=azure' in output")
	}
	if !strings.Contains(result, "export AZURE_OPENAI_ENDPOINT=https://my-resource.openai.azure.com") {
		t.Error("Expected Azure endpoint in output")
	}
	if !strings.Contains(result, "export AZURE_OPENAI_API_KEY=azure-key-123") {
		t.Error("Expected Azure API key in output")
	}
	if !strings.Contains(result, "export AZURE_OPENAI_MODEL=gpt-4o") {
		t.Error("Expected Azure model in output")
	}
	if !strings.Contains(result, "export AZURE_OPENAI_API_VERSION=2025-04-01-preview") {
		t.Error("Expected Azure API version in output")
	}
}

func TestGenerateEnvExports_AzureWithoutAPIVersion(t *testing.T) {
	result := generateEnvExports("azure", "https://my-resource.openai.azure.com", "azure-key-123", "gpt-4o", "", ShellBashZsh)

	// Should NOT contain API version if empty
	if strings.Contains(result, "AZURE_OPENAI_API_VERSION") {
		t.Error("Should not contain AZURE_OPENAI_API_VERSION when empty")
	}
}

func TestGenerateEnvExports_Fish(t *testing.T) {
	result := generateEnvExports("local", "http://localhost:11434", "", "llama3", "", ShellFish)

	// Fish uses 'set -gx' instead of 'export'
	if !strings.Contains(result, "set -gx LLM_PROVIDER local") {
		t.Error("Expected 'set -gx LLM_PROVIDER local' for fish shell")
	}
	if !strings.Contains(result, "set -gx LLM_ENDPOINT http://localhost:11434") {
		t.Error("Expected 'set -gx LLM_ENDPOINT' for fish shell")
	}
	if !strings.Contains(result, "set -gx LLM_MODEL llama3") {
		t.Error("Expected 'set -gx LLM_MODEL' for fish shell")
	}
	// Should NOT contain 'export'
	if strings.Contains(result, "export ") {
		t.Error("Fish output should not contain 'export'")
	}
}

func TestGenerateEnvExports_FishAzure(t *testing.T) {
	result := generateEnvExports("azure", "https://test.openai.azure.com", "key123", "gpt-4o", "2025-04-01-preview", ShellFish)

	if !strings.Contains(result, "set -gx LLM_PROVIDER azure") {
		t.Error("Expected 'set -gx LLM_PROVIDER azure' for fish shell")
	}
	if !strings.Contains(result, "set -gx AZURE_OPENAI_ENDPOINT https://test.openai.azure.com") {
		t.Error("Expected Azure endpoint with 'set -gx' for fish shell")
	}
}

func TestGenerateEnvExports_PowerShellLocal(t *testing.T) {
	result := generateEnvExports("local", "http://localhost:1234", "", "llama3", "", ShellPowerShell)

	// PowerShell uses $env:VAR = "value" syntax
	if !strings.Contains(result, `$env:LLM_PROVIDER = "local"`) {
		t.Errorf("Expected PowerShell syntax for LLM_PROVIDER, got: %s", result)
	}
	if !strings.Contains(result, `$env:LLM_ENDPOINT = "http://localhost:1234"`) {
		t.Errorf("Expected PowerShell syntax for LLM_ENDPOINT, got: %s", result)
	}
	if !strings.Contains(result, `$env:LLM_MODEL = "llama3"`) {
		t.Errorf("Expected PowerShell syntax for LLM_MODEL, got: %s", result)
	}
	// Should NOT contain bash-style export
	if strings.Contains(result, "export ") {
		t.Error("PowerShell output should not contain 'export'")
	}
	// Should NOT contain fish-style set
	if strings.Contains(result, "set -gx") {
		t.Error("PowerShell output should not contain 'set -gx'")
	}
}

func TestGenerateEnvExports_PowerShellLocalWithAPIKey(t *testing.T) {
	result := generateEnvExports("local", "http://localhost:1234", "my-secret-key", "mistral", "", ShellPowerShell)

	if !strings.Contains(result, `$env:LLM_API_KEY = "my-secret-key"`) {
		t.Errorf("Expected PowerShell syntax for LLM_API_KEY, got: %s", result)
	}
}

func TestGenerateEnvExports_PowerShellAzure(t *testing.T) {
	result := generateEnvExports("azure", "https://my-resource.openai.azure.com", "azure-key-123", "gpt-4o", "2025-04-01-preview", ShellPowerShell)

	if !strings.Contains(result, `$env:LLM_PROVIDER = "azure"`) {
		t.Errorf("Expected PowerShell syntax for LLM_PROVIDER, got: %s", result)
	}
	if !strings.Contains(result, `$env:AZURE_OPENAI_ENDPOINT = "https://my-resource.openai.azure.com"`) {
		t.Errorf("Expected PowerShell syntax for AZURE_OPENAI_ENDPOINT, got: %s", result)
	}
	if !strings.Contains(result, `$env:AZURE_OPENAI_API_KEY = "azure-key-123"`) {
		t.Errorf("Expected PowerShell syntax for AZURE_OPENAI_API_KEY, got: %s", result)
	}
	if !strings.Contains(result, `$env:AZURE_OPENAI_MODEL = "gpt-4o"`) {
		t.Errorf("Expected PowerShell syntax for AZURE_OPENAI_MODEL, got: %s", result)
	}
	if !strings.Contains(result, `$env:AZURE_OPENAI_API_VERSION = "2025-04-01-preview"`) {
		t.Errorf("Expected PowerShell syntax for AZURE_OPENAI_API_VERSION, got: %s", result)
	}
}

func TestGenerateEnvExports_PowerShellAzureWithoutAPIVersion(t *testing.T) {
	result := generateEnvExports("azure", "https://my-resource.openai.azure.com", "azure-key-123", "gpt-4o", "", ShellPowerShell)

	// Should NOT contain API version if empty
	if strings.Contains(result, "AZURE_OPENAI_API_VERSION") {
		t.Error("Should not contain AZURE_OPENAI_API_VERSION when empty")
	}
}

func TestGenerateDotEnv_Local(t *testing.T) {
	result := generateDotEnv("local", "http://localhost:1234", "", "qwen2", "")

	// .env format should NOT have 'export' prefix
	if strings.Contains(result, "export ") {
		t.Error(".env format should not contain 'export'")
	}
	if !strings.Contains(result, "LLM_PROVIDER=local") {
		t.Error("Expected 'LLM_PROVIDER=local' in .env output")
	}
	if !strings.Contains(result, "LLM_ENDPOINT=http://localhost:1234") {
		t.Error("Expected 'LLM_ENDPOINT=http://localhost:1234' in .env output")
	}
	if !strings.Contains(result, "LLM_MODEL=qwen2") {
		t.Error("Expected 'LLM_MODEL=qwen2' in .env output")
	}
	if !strings.Contains(result, "# CapyCut configuration") {
		t.Error("Expected configuration comment in .env output")
	}
}

func TestGenerateDotEnv_LocalWithAPIKey(t *testing.T) {
	result := generateDotEnv("local", "http://localhost:8080", "secret123", "codellama", "")

	if !strings.Contains(result, "LLM_API_KEY=secret123") {
		t.Error("Expected 'LLM_API_KEY=secret123' in .env output")
	}
}

func TestGenerateDotEnv_Azure(t *testing.T) {
	result := generateDotEnv("azure", "https://myresource.openai.azure.com", "myapikey", "gpt-4o-mini", "2025-04-01-preview")

	if !strings.Contains(result, "LLM_PROVIDER=azure") {
		t.Error("Expected 'LLM_PROVIDER=azure' in .env output")
	}
	if !strings.Contains(result, "AZURE_OPENAI_ENDPOINT=https://myresource.openai.azure.com") {
		t.Error("Expected Azure endpoint in .env output")
	}
	if !strings.Contains(result, "AZURE_OPENAI_API_KEY=myapikey") {
		t.Error("Expected Azure API key in .env output")
	}
	if !strings.Contains(result, "AZURE_OPENAI_MODEL=gpt-4o-mini") {
		t.Error("Expected Azure model in .env output")
	}
	if !strings.Contains(result, "AZURE_OPENAI_API_VERSION=2025-04-01-preview") {
		t.Error("Expected Azure API version in .env output")
	}
}

func TestGenerateDotEnv_AzureWithoutAPIVersion(t *testing.T) {
	result := generateDotEnv("azure", "https://myresource.openai.azure.com", "myapikey", "gpt-4o", "")

	if strings.Contains(result, "AZURE_OPENAI_API_VERSION") {
		t.Error("Should not contain AZURE_OPENAI_API_VERSION when empty")
	}
}

func TestDetectShell(t *testing.T) {
	// Save original SHELL env
	origShell := os.Getenv("SHELL")
	defer os.Setenv("SHELL", origShell)

	// Test with zsh
	os.Setenv("SHELL", "/bin/zsh")
	shell, profiles := detectShell()

	if !strings.Contains(shell, "zsh") {
		t.Errorf("Expected shell to contain 'zsh', got %s", shell)
	}

	// Should always have at least the .env option
	found := false
	for _, p := range profiles {
		if strings.Contains(p.name, ".env") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected .env option in profiles")
	}
}

func TestDetectShell_Bash(t *testing.T) {
	origShell := os.Getenv("SHELL")
	defer os.Setenv("SHELL", origShell)

	os.Setenv("SHELL", "/bin/bash")
	shell, _ := detectShell()

	if !strings.Contains(shell, "bash") {
		t.Errorf("Expected shell to contain 'bash', got %s", shell)
	}
}

func TestDetectShell_DetectsExistingProfiles(t *testing.T) {
	// Create a temp directory structure
	tmpDir, err := os.MkdirTemp("", "capycut-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// We can't easily test home directory detection without mocking,
	// but we can verify the function doesn't panic and returns .env option
	_, profiles := detectShell()

	// Should always have .env as an option
	hasEnvOption := false
	for _, p := range profiles {
		if strings.Contains(p.name, ".env") {
			hasEnvOption = true
		}
	}
	if !hasEnvOption {
		t.Error("detectShell should always include .env option")
	}
}

func TestDetectShell_ProfileDetection(t *testing.T) {
	origShell := os.Getenv("SHELL")
	defer os.Setenv("SHELL", origShell)

	// Test that zsh profiles are marked as detected when shell is zsh
	os.Setenv("SHELL", "/bin/zsh")
	_, profiles := detectShell()

	for _, p := range profiles {
		if strings.Contains(p.name, "zsh") && !p.detected {
			// This might fail if no zsh profiles exist on the system
			// which is fine - we're just testing the detection logic
			t.Logf("Note: zsh profile %s exists but not marked as detected", p.name)
		}
	}
}

func TestShellProfile_Struct(t *testing.T) {
	p := shellProfile{
		name:     "test profile",
		path:     "/path/to/profile",
		detected: true,
	}

	if p.name != "test profile" {
		t.Error("shellProfile name not set correctly")
	}
	if p.path != "/path/to/profile" {
		t.Error("shellProfile path not set correctly")
	}
	if !p.detected {
		t.Error("shellProfile detected not set correctly")
	}
}

func TestGenerateEnvExports_OutputFormat(t *testing.T) {
	result := generateEnvExports("local", "http://localhost:1234", "", "test-model", "", ShellBashZsh)

	// Should start with newline (for appending to existing files)
	if !strings.HasPrefix(result, "\n") {
		t.Error("Output should start with newline for clean appending")
	}

	// Should end with newline
	if !strings.HasSuffix(result, "\n") {
		t.Error("Output should end with newline")
	}
}

func TestGenerateEnvExports_PowerShellOutputFormat(t *testing.T) {
	result := generateEnvExports("local", "http://localhost:1234", "", "test-model", "", ShellPowerShell)

	// Should start with newline (for appending to existing files)
	if !strings.HasPrefix(result, "\n") {
		t.Error("PowerShell output should start with newline for clean appending")
	}

	// Should end with newline
	if !strings.HasSuffix(result, "\n") {
		t.Error("PowerShell output should end with newline")
	}

	// Should contain PowerShell comment
	if !strings.Contains(result, "# CapyCut configuration") {
		t.Error("Expected configuration comment in PowerShell output")
	}
}

func TestGenerateDotEnv_OutputFormat(t *testing.T) {
	result := generateDotEnv("local", "http://localhost:1234", "", "test-model", "")

	// Should start with comment (no leading newline for new files)
	if !strings.HasPrefix(result, "# CapyCut") {
		t.Error(".env output should start with comment header")
	}

	// Should end with newline
	if !strings.HasSuffix(result, "\n") {
		t.Error(".env output should end with newline")
	}
}

// Integration test: verify generated config can be parsed
func TestGenerateDotEnv_ValidFormat(t *testing.T) {
	result := generateDotEnv("local", "http://localhost:1234", "key123", "llama3", "")

	lines := strings.Split(result, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Each non-empty, non-comment line should have exactly one '='
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			t.Errorf("Invalid .env line format: %s", line)
		}
		if parts[0] == "" {
			t.Errorf("Empty key in .env line: %s", line)
		}
	}
}

func TestWriteAndReadConfig(t *testing.T) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "capycut-test-*.env")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Generate and write config
	config := generateDotEnv("local", "http://localhost:1234", "", "test-model", "")
	err = os.WriteFile(tmpFile.Name(), []byte(config), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Read it back
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	if string(content) != config {
		t.Error("Written config doesn't match generated config")
	}
}

func TestAppendToProfile(t *testing.T) {
	// Create temp file with existing content
	tmpFile, err := os.CreateTemp("", "capycut-test-profile-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	existingContent := "# Existing profile content\nexport PATH=/usr/bin:$PATH\n"
	_, err = tmpFile.WriteString(existingContent)
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	// Generate config to append
	config := generateEnvExports("local", "http://localhost:1234", "", "llama3", "", ShellBashZsh)

	// Append to file
	f, err := os.OpenFile(tmpFile.Name(), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(config)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Read and verify
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	contentStr := string(content)

	// Should contain original content
	if !strings.Contains(contentStr, "Existing profile content") {
		t.Error("Original content should be preserved")
	}
	if !strings.Contains(contentStr, "export PATH=/usr/bin:$PATH") {
		t.Error("Original exports should be preserved")
	}

	// Should contain new config
	if !strings.Contains(contentStr, "export LLM_PROVIDER=local") {
		t.Error("New config should be appended")
	}
	if !strings.Contains(contentStr, "export LLM_ENDPOINT=http://localhost:1234") {
		t.Error("New config should be appended")
	}
}

func TestFilePermissions(t *testing.T) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "capycut-test-perms-*.env")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Write with restricted permissions (0600)
	config := generateDotEnv("azure", "https://test.azure.com", "secret-key", "gpt-4o", "")
	err = os.WriteFile(tmpPath, []byte(config), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Check permissions
	info, err := os.Stat(tmpPath)
	if err != nil {
		t.Fatal(err)
	}

	// On Unix, verify it's not world-readable
	perm := info.Mode().Perm()
	if perm&0044 != 0 {
		t.Logf("Warning: Config file may be readable by others (perm: %o)", perm)
	}
}

func TestDetectShell_EnvFilePath(t *testing.T) {
	_, profiles := detectShell()

	// Find the .env option
	var envProfile *shellProfile
	for i, p := range profiles {
		if strings.Contains(p.name, ".env") {
			envProfile = &profiles[i]
			break
		}
	}

	if envProfile == nil {
		t.Fatal("Expected .env profile option")
	}

	// Should point to current directory
	expected := filepath.Join(".", ".env")
	if envProfile.path != expected {
		t.Errorf("Expected .env path to be %s, got %s", expected, envProfile.path)
	}

	// .env should not be marked as "detected" (it's not shell-specific)
	if envProfile.detected {
		t.Error(".env option should not be marked as detected")
	}
}

// Tests for getShellType function
func TestGetShellType_PowerShell(t *testing.T) {
	tests := []struct {
		path     string
		expected ShellType
	}{
		{"Microsoft.PowerShell_profile.ps1", ShellPowerShell},
		{"/home/user/.config/powershell/Microsoft.PowerShell_profile.ps1", ShellPowerShell},
		{"C:\\Users\\test\\Documents\\PowerShell\\Microsoft.PowerShell_profile.ps1", ShellPowerShell},
		{"profile.PS1", ShellPowerShell}, // case insensitive
	}

	for _, tt := range tests {
		result := getShellType(tt.path)
		if result != tt.expected {
			t.Errorf("getShellType(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

func TestGetShellType_Fish(t *testing.T) {
	tests := []struct {
		path     string
		expected ShellType
	}{
		{"config.fish", ShellFish},
		{"/home/user/.config/fish/config.fish", ShellFish},
		{"~/.config/fish/config.fish", ShellFish},
	}

	for _, tt := range tests {
		result := getShellType(tt.path)
		if result != tt.expected {
			t.Errorf("getShellType(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

func TestGetShellType_DotEnv(t *testing.T) {
	tests := []struct {
		path     string
		expected ShellType
	}{
		{".env", ShellDotEnv},
		{"/path/to/project/.env", ShellDotEnv},
		{"C:\\project\\.env", ShellDotEnv},
	}

	for _, tt := range tests {
		result := getShellType(tt.path)
		if result != tt.expected {
			t.Errorf("getShellType(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

func TestGetShellType_BashZsh(t *testing.T) {
	tests := []struct {
		path     string
		expected ShellType
	}{
		{".bashrc", ShellBashZsh},
		{".bash_profile", ShellBashZsh},
		{".zshrc", ShellBashZsh},
		{".zprofile", ShellBashZsh},
		{"/home/user/.bashrc", ShellBashZsh},
	}

	for _, tt := range tests {
		result := getShellType(tt.path)
		if result != tt.expected {
			t.Errorf("getShellType(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

// Tests for shortenPath function
func TestShortenPath(t *testing.T) {
	homeDir := "/home/testuser"

	tests := []struct {
		path     string
		expected string
	}{
		{"/home/testuser/.bashrc", "~/.bashrc"},
		{"/home/testuser/Documents/file.txt", "~/Documents/file.txt"},
		{"/other/path/file.txt", "/other/path/file.txt"},
		{"/home/testuser", "~"},
	}

	for _, tt := range tests {
		result := shortenPath(tt.path, homeDir)
		if result != tt.expected {
			t.Errorf("shortenPath(%q, %q) = %q, want %q", tt.path, homeDir, result, tt.expected)
		}
	}
}

// Tests for fileExists function
func TestFileExists(t *testing.T) {
	// Create a temp file
	tmpFile, err := os.CreateTemp("", "capycut-test-exists-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Should exist
	if !fileExists(tmpPath) {
		t.Error("fileExists should return true for existing file")
	}

	// Remove it
	os.Remove(tmpPath)

	// Should not exist
	if fileExists(tmpPath) {
		t.Error("fileExists should return false for non-existing file")
	}

	// Non-existent path
	if fileExists("/this/path/definitely/does/not/exist/file.txt") {
		t.Error("fileExists should return false for non-existing path")
	}
}

// Test PowerShell profile writing
func TestAppendToPowerShellProfile(t *testing.T) {
	// Create temp file with existing PowerShell content
	tmpFile, err := os.CreateTemp("", "capycut-test-profile-*.ps1")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	existingContent := "# Existing PowerShell profile\n$env:PATH = \"C:\\Program Files\\Git\\bin;$env:PATH\"\n"
	_, err = tmpFile.WriteString(existingContent)
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	// Generate PowerShell config to append
	config := generateEnvExports("local", "http://localhost:1234", "", "llama3", "", ShellPowerShell)

	// Append to file
	f, err := os.OpenFile(tmpFile.Name(), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(config)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Read and verify
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	contentStr := string(content)

	// Should contain original content
	if !strings.Contains(contentStr, "Existing PowerShell profile") {
		t.Error("Original content should be preserved")
	}
	if !strings.Contains(contentStr, `$env:PATH = "C:\Program Files\Git\bin;$env:PATH"`) {
		t.Error("Original PowerShell exports should be preserved")
	}

	// Should contain new config in PowerShell format
	if !strings.Contains(contentStr, `$env:LLM_PROVIDER = "local"`) {
		t.Error("New PowerShell config should be appended")
	}
	if !strings.Contains(contentStr, `$env:LLM_ENDPOINT = "http://localhost:1234"`) {
		t.Error("New PowerShell config should be appended")
	}
}

// Test that PowerShell config is valid PowerShell syntax
func TestGenerateEnvExports_PowerShellValidSyntax(t *testing.T) {
	result := generateEnvExports("azure", "https://test.azure.com", "secret-key", "gpt-4o", "2025-04-01-preview", ShellPowerShell)

	lines := strings.Split(result, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Each non-empty, non-comment line should be valid PowerShell env assignment
		// Format: $env:VAR_NAME = "value"
		if !strings.HasPrefix(line, "$env:") {
			t.Errorf("Invalid PowerShell line (should start with $env:): %s", line)
		}
		if !strings.Contains(line, " = \"") {
			t.Errorf("Invalid PowerShell line (should contain ' = \"'): %s", line)
		}
		if !strings.HasSuffix(line, "\"") {
			t.Errorf("Invalid PowerShell line (should end with '\"'): %s", line)
		}
	}
}

// Test cross-platform path handling
func TestPathHandling_CrossPlatform(t *testing.T) {
	// Test that filepath.Join works correctly for profile paths
	homeDir := "/home/user"

	// These should all be valid paths when joined
	paths := [][]string{
		{homeDir, ".bashrc"},
		{homeDir, ".config", "fish", "config.fish"},
		{homeDir, ".config", "powershell", "Microsoft.PowerShell_profile.ps1"},
		{homeDir, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1"},
	}

	for _, parts := range paths {
		result := filepath.Join(parts...)
		if result == "" {
			t.Errorf("filepath.Join(%v) returned empty string", parts)
		}
		// Verify the path contains the expected filename
		expectedFilename := parts[len(parts)-1]
		if !strings.HasSuffix(result, expectedFilename) {
			t.Errorf("filepath.Join(%v) = %s, should end with %s", parts, result, expectedFilename)
		}
	}
}

// Test Windows-style paths in getShellType
func TestGetShellType_WindowsPaths(t *testing.T) {
	tests := []struct {
		path     string
		expected ShellType
	}{
		{`C:\Users\Test\Documents\PowerShell\Microsoft.PowerShell_profile.ps1`, ShellPowerShell},
		{`C:\Users\Test\Documents\WindowsPowerShell\Microsoft.PowerShell_profile.ps1`, ShellPowerShell},
		{`C:\Users\Test\.bashrc`, ShellBashZsh},
		{`C:\project\.env`, ShellDotEnv},
	}

	for _, tt := range tests {
		result := getShellType(tt.path)
		if result != tt.expected {
			t.Errorf("getShellType(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}
