package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"capycut/ai"
	"capycut/video"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/creativeprojects/go-selfupdate"
	"github.com/joho/godotenv"
)

// Build info - set via ldflags
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6B6B")).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4ECDC4")).
			MarginBottom(1)

	successStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#95E1A3"))

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6B6B"))

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A8A8A8"))

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#4ECDC4")).
			Padding(1, 2).
			MarginTop(1).
			MarginBottom(1)

	capybaraLogo = `
    ╭─────────────────────────────────────╮
    │  🦫 CapyCut - AI Video Clipper      │
    ╰─────────────────────────────────────╯`
)

// CLI flags
var (
	versionFlag      bool
	shortVersionFlag bool
	debugFlag        bool
	helpFlag         bool
	setupFlag        bool
	updateFlag       bool
	providerFlag     string
	fileFlag         string
	promptFlag       string
	outputFlag       string
)

func init() {
	flag.BoolVar(&versionFlag, "version", false, "Print version information")
	flag.BoolVar(&shortVersionFlag, "v", false, "Print version information (short)")
	flag.BoolVar(&debugFlag, "debug", false, "Enable debug output")
	flag.BoolVar(&helpFlag, "help", false, "Show help message")
	flag.BoolVar(&helpFlag, "h", false, "Show help message (short)")
	flag.BoolVar(&setupFlag, "setup", false, "Run interactive setup wizard")
	flag.BoolVar(&updateFlag, "update", false, "Update capycut to the latest version")
	flag.StringVar(&providerFlag, "provider", "", "LLM provider: 'local' or 'azure'")
	flag.StringVar(&fileFlag, "file", "", "Path to video file")
	flag.StringVar(&fileFlag, "f", "", "Path to video file (short)")
	flag.StringVar(&promptFlag, "prompt", "", "Clip description (e.g., 'first 2 minutes')")
	flag.StringVar(&promptFlag, "p", "", "Clip description (short)")
	flag.StringVar(&outputFlag, "output", "", "Output file path (optional)")
	flag.StringVar(&outputFlag, "o", "", "Output file path (short)")
}

func printHelp() {
	help := `
🦫 CapyCut - AI-powered video clipper

USAGE:
    capycut [OPTIONS]
    capycut --file <video> --prompt <description>

OPTIONS:
    -f, --file <path>       Path to video file
    -p, --prompt <text>     Clip description in natural language
                            Examples:
                              "first 2 minutes"
                              "from 3:00 to 5:30"
                              "last 45 seconds"
                              "start at 1:23, end at 4:56"
    -o, --output <path>     Output file path (optional, auto-generated if not set)
    
    --provider <name>       LLM provider: 'local' or 'azure' (overrides LLM_PROVIDER env var)
    
    --setup                 Run interactive setup wizard to configure CapyCut
    --update                Update capycut to the latest version
    --debug                 Enable debug output
    -v, --version           Print version information
    -h, --help              Show this help message

ENVIRONMENT VARIABLES:

  Provider Selection:
    LLM_PROVIDER            Set to 'local' or 'azure' (or use --provider flag)
                            Auto-detects if not set: uses 'local' if LLM_ENDPOINT is set,
                            otherwise defaults to 'azure'

  Local LLM (LM Studio, Ollama, etc.):
    LLM_ENDPOINT            API endpoint (e.g., http://localhost:1234)
    LLM_MODEL               Model name to use
    LLM_API_KEY             API key (optional for most local LLMs)

  Azure OpenAI:
    AZURE_OPENAI_ENDPOINT   Your Azure OpenAI endpoint URL
    AZURE_OPENAI_API_KEY    Your Azure OpenAI API key
    AZURE_OPENAI_MODEL      Model deployment name (e.g., gpt-4o)
    AZURE_OPENAI_API_VERSION  API version (optional, defaults to 2025-04-01-preview)

  Debug:
    CAPYCUT_DEBUG           Set to any value to enable debug output

EXAMPLES:
    # Interactive mode (select file and enter prompt via UI)
    capycut

    # Non-interactive mode with arguments
    capycut -f video.mp4 -p "first 2 minutes"
    capycut --file video.mp4 --prompt "from 1:00 to 3:30" --output clip.mp4

    # Use specific LLM provider via flag
    capycut --provider local -f video.mp4 -p "last 30 seconds"
    capycut --provider azure -f video.mp4 -p "first 5 minutes"

    # Or set provider via environment variable
    export LLM_PROVIDER=local
    capycut -f video.mp4 -p "first 2 minutes"

    # Local LLM setup example (LM Studio)
    export LLM_PROVIDER=local
    export LLM_ENDPOINT=http://localhost:1234
    export LLM_MODEL=my-model
    capycut

    # Azure setup example
    export LLM_PROVIDER=azure
    export AZURE_OPENAI_ENDPOINT=https://my-resource.openai.azure.com
    export AZURE_OPENAI_API_KEY=my-key
    export AZURE_OPENAI_MODEL=gpt-4o
    capycut

For more info: https://github.com/harmonyvt/capycut
`
	fmt.Println(help)
}

// Shell profile info
type shellProfile struct {
	name     string
	path     string
	detected bool
}

// ShellType represents the type of shell for config generation
type ShellType int

const (
	ShellBashZsh ShellType = iota
	ShellFish
	ShellPowerShell
	ShellDotEnv
)

// detectShell detects the current shell and returns available profile files
func detectShell() (string, []shellProfile) {
	shell := os.Getenv("SHELL")
	// On Windows, also check ComSpec and PSModulePath for PowerShell detection
	if runtime.GOOS == "windows" && shell == "" {
		if os.Getenv("PSModulePath") != "" {
			shell = "powershell"
		} else {
			shell = os.Getenv("ComSpec") // Usually cmd.exe
		}
	}

	homeDir, _ := os.UserHomeDir()
	profiles := []shellProfile{}

	if runtime.GOOS != "windows" {
		// Unix-like systems: check for zsh profiles
		zshrc := filepath.Join(homeDir, ".zshrc")
		zprofile := filepath.Join(homeDir, ".zprofile")
		if fileExists(zshrc) {
			profiles = append(profiles, shellProfile{"zsh (~/.zshrc)", zshrc, strings.Contains(shell, "zsh")})
		}
		if fileExists(zprofile) {
			profiles = append(profiles, shellProfile{"zsh (~/.zprofile)", zprofile, strings.Contains(shell, "zsh")})
		}

		// Check for bash profiles
		bashrc := filepath.Join(homeDir, ".bashrc")
		bashProfile := filepath.Join(homeDir, ".bash_profile")
		if fileExists(bashrc) {
			profiles = append(profiles, shellProfile{"bash (~/.bashrc)", bashrc, strings.Contains(shell, "bash")})
		}
		if fileExists(bashProfile) {
			profiles = append(profiles, shellProfile{"bash (~/.bash_profile)", bashProfile, strings.Contains(shell, "bash")})
		}

		// Check for fish config
		fishConfig := filepath.Join(homeDir, ".config", "fish", "config.fish")
		if fileExists(fishConfig) {
			profiles = append(profiles, shellProfile{"fish (~/.config/fish/config.fish)", fishConfig, strings.Contains(shell, "fish")})
		}

		// PowerShell 7+ on Unix (cross-platform location)
		pwshProfile := filepath.Join(homeDir, ".config", "powershell", "Microsoft.PowerShell_profile.ps1")
		if fileExists(pwshProfile) {
			profiles = append(profiles, shellProfile{"PowerShell 7+ (~/.config/powershell/profile.ps1)", pwshProfile, strings.Contains(shell, "pwsh")})
		} else if isPowerShellAvailable() {
			// Offer to create PowerShell profile
			profiles = append(profiles, shellProfile{"PowerShell (create new profile)", pwshProfile, false})
		}
	} else {
		// Windows-specific profile locations
		documentsDir := filepath.Join(homeDir, "Documents")

		// PowerShell 7 on Windows
		pwsh7Profile := filepath.Join(documentsDir, "PowerShell", "Microsoft.PowerShell_profile.ps1")
		if fileExists(pwsh7Profile) {
			profiles = append(profiles, shellProfile{
				fmt.Sprintf("PowerShell 7 (%s)", shortenPath(pwsh7Profile, homeDir)),
				pwsh7Profile,
				strings.Contains(strings.ToLower(shell), "pwsh"),
			})
		}

		// Windows PowerShell 5.x
		winPwshProfile := filepath.Join(documentsDir, "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1")
		if fileExists(winPwshProfile) {
			profiles = append(profiles, shellProfile{
				fmt.Sprintf("Windows PowerShell (%s)", shortenPath(winPwshProfile, homeDir)),
				winPwshProfile,
				strings.Contains(strings.ToLower(shell), "powershell"),
			})
		}

		// Git Bash on Windows
		gitBashProfile := filepath.Join(homeDir, ".bashrc")
		if fileExists(gitBashProfile) {
			profiles = append(profiles, shellProfile{
				fmt.Sprintf("Git Bash (%s)", shortenPath(gitBashProfile, homeDir)),
				gitBashProfile,
				strings.Contains(strings.ToLower(shell), "bash"),
			})
		}

		// If no PowerShell profile exists, offer to create one
		hasPwshProfile := false
		for _, p := range profiles {
			if strings.Contains(p.name, "PowerShell") {
				hasPwshProfile = true
				break
			}
		}
		if !hasPwshProfile {
			// Default to PowerShell 7 location
			newProfile := pwsh7Profile
			profiles = append(profiles, shellProfile{
				fmt.Sprintf("PowerShell (create new: %s)", shortenPath(newProfile, homeDir)),
				newProfile,
				true, // Mark as detected on Windows since PowerShell is usually available
			})
		}
	}

	// Always offer .env option (works on all platforms)
	envFile := filepath.Join(".", ".env")
	profiles = append(profiles, shellProfile{".env file (current directory)", envFile, false})

	return shell, profiles
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// isPowerShellAvailable checks if PowerShell is available on the system
func isPowerShellAvailable() bool {
	if runtime.GOOS == "windows" {
		return true // PowerShell is always available on Windows
	}
	// Check common Unix locations for pwsh
	locations := []string{
		"/usr/local/bin/pwsh",
		"/usr/bin/pwsh",
		"/opt/microsoft/powershell/7/pwsh",
	}
	for _, loc := range locations {
		if fileExists(loc) {
			return true
		}
	}
	return false
}

// shortenPath replaces home directory with ~ for display
func shortenPath(path, homeDir string) string {
	if runtime.GOOS == "windows" {
		// On Windows, use %USERPROFILE% style or just show relative path
		if strings.HasPrefix(path, homeDir) {
			return "~" + strings.TrimPrefix(path, homeDir)
		}
		return path
	}
	if strings.HasPrefix(path, homeDir) {
		return "~" + strings.TrimPrefix(path, homeDir)
	}
	return path
}

// getShellType determines the shell type from a profile path
func getShellType(profilePath string) ShellType {
	lower := strings.ToLower(profilePath)
	if strings.HasSuffix(lower, ".ps1") {
		return ShellPowerShell
	}
	if strings.Contains(lower, "fish") {
		return ShellFish
	}
	if strings.HasSuffix(lower, ".env") {
		return ShellDotEnv
	}
	return ShellBashZsh
}

// generateEnvExports generates export statements for the given config
func generateEnvExports(provider, endpoint, apiKey, model, apiVersion string, shellType ShellType) string {
	var lines []string

	lines = append(lines, "")
	lines = append(lines, "# CapyCut configuration")

	var envVars []struct{ key, value string }

	if provider == "local" {
		envVars = append(envVars, struct{ key, value string }{"LLM_PROVIDER", "local"})
		envVars = append(envVars, struct{ key, value string }{"LLM_ENDPOINT", endpoint})
		envVars = append(envVars, struct{ key, value string }{"LLM_MODEL", model})
		if apiKey != "" {
			envVars = append(envVars, struct{ key, value string }{"LLM_API_KEY", apiKey})
		}
	} else {
		envVars = append(envVars, struct{ key, value string }{"LLM_PROVIDER", "azure"})
		envVars = append(envVars, struct{ key, value string }{"AZURE_OPENAI_ENDPOINT", endpoint})
		envVars = append(envVars, struct{ key, value string }{"AZURE_OPENAI_API_KEY", apiKey})
		envVars = append(envVars, struct{ key, value string }{"AZURE_OPENAI_MODEL", model})
		if apiVersion != "" {
			envVars = append(envVars, struct{ key, value string }{"AZURE_OPENAI_API_VERSION", apiVersion})
		}
	}

	for _, env := range envVars {
		switch shellType {
		case ShellPowerShell:
			// PowerShell: $env:VAR_NAME = "value"
			lines = append(lines, fmt.Sprintf(`$env:%s = "%s"`, env.key, env.value))
		case ShellFish:
			// Fish: set -gx VAR_NAME value
			lines = append(lines, fmt.Sprintf("set -gx %s %s", env.key, env.value))
		default:
			// Bash/Zsh: export VAR_NAME=value
			lines = append(lines, fmt.Sprintf("export %s=%s", env.key, env.value))
		}
	}

	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

// generateDotEnv generates .env file content
func generateDotEnv(provider, endpoint, apiKey, model, apiVersion string) string {
	var lines []string

	lines = append(lines, "# CapyCut configuration")

	if provider == "local" {
		lines = append(lines, "LLM_PROVIDER=local")
		lines = append(lines, fmt.Sprintf("LLM_ENDPOINT=%s", endpoint))
		lines = append(lines, fmt.Sprintf("LLM_MODEL=%s", model))
		if apiKey != "" {
			lines = append(lines, fmt.Sprintf("LLM_API_KEY=%s", apiKey))
		}
	} else {
		lines = append(lines, "LLM_PROVIDER=azure")
		lines = append(lines, fmt.Sprintf("AZURE_OPENAI_ENDPOINT=%s", endpoint))
		lines = append(lines, fmt.Sprintf("AZURE_OPENAI_API_KEY=%s", apiKey))
		lines = append(lines, fmt.Sprintf("AZURE_OPENAI_MODEL=%s", model))
		if apiVersion != "" {
			lines = append(lines, fmt.Sprintf("AZURE_OPENAI_API_VERSION=%s", apiVersion))
		}
	}
	lines = append(lines, "")

	return strings.Join(lines, "\n")
}

// GitHub repository for updates
const (
	repoOwner = "harmonyvt"
	repoName  = "capycut"
)

func runSelfUpdate() error {
	fmt.Println(infoStyle.Render("Checking for updates..."))

	// Skip update check for dev version
	if version == "dev" {
		fmt.Println(infoStyle.Render("Running development version, skipping update check."))
		return nil
	}

	source, err := selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
	if err != nil {
		return fmt.Errorf("failed to create update source: %w", err)
	}

	updater, err := selfupdate.NewUpdater(selfupdate.Config{
		Source:    source,
		Validator: nil, // No signature validation for now
	})
	if err != nil {
		return fmt.Errorf("failed to create updater: %w", err)
	}

	latest, found, err := updater.DetectLatest(context.Background(), selfupdate.NewRepositorySlug(repoOwner, repoName))
	if err != nil {
		return fmt.Errorf("failed to detect latest version: %w", err)
	}
	if !found {
		fmt.Println(infoStyle.Render("No release found."))
		return nil
	}

	currentVersion := version
	if strings.HasPrefix(currentVersion, "v") {
		currentVersion = currentVersion[1:]
	}

	if !latest.GreaterThan(currentVersion) {
		fmt.Println(successStyle.Render(fmt.Sprintf("Already up to date! (version %s)", version)))
		return nil
	}

	fmt.Println(subtitleStyle.Render(fmt.Sprintf("New version available: %s (current: %s)", latest.Version(), version)))
	fmt.Println(infoStyle.Render("Downloading update..."))

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	if err := updater.UpdateTo(context.Background(), latest, exe); err != nil {
		return fmt.Errorf("failed to update: %w", err)
	}

	fmt.Println(successStyle.Render(fmt.Sprintf("Successfully updated to version %s!", latest.Version())))
	fmt.Println(infoStyle.Render("Please restart capycut to use the new version."))
	return nil
}

func runSetupWizard() {
	fmt.Println(titleStyle.Render(capybaraLogo))
	fmt.Println(subtitleStyle.Render("Welcome to CapyCut Setup! Let's configure your LLM provider.\n"))

	// Step 1: Choose provider
	var provider string
	providerSelect := huh.NewSelect[string]().
		Title("Which LLM provider would you like to use?").
		Options(
			huh.NewOption("Local LLM (LM Studio, Ollama, etc.) - Free & Private", "local"),
			huh.NewOption("Azure OpenAI - Cloud-based", "azure"),
		).
		Value(&provider)

	err := huh.NewForm(huh.NewGroup(providerSelect)).
		WithTheme(huh.ThemeCatppuccin()).
		Run()

	if err != nil {
		if err == huh.ErrUserAborted {
			fmt.Println(infoStyle.Render("Setup cancelled."))
			return
		}
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		return
	}

	var endpoint, apiKey, model, apiVersion string

	if provider == "local" {
		// Local LLM setup
		var localProvider string
		localSelect := huh.NewSelect[string]().
			Title("Which local LLM server are you using?").
			Options(
				huh.NewOption("LM Studio (default port: 1234)", "lmstudio"),
				huh.NewOption("Ollama (default port: 11434)", "ollama"),
				huh.NewOption("Other OpenAI-compatible server", "other"),
			).
			Value(&localProvider)

		err = huh.NewForm(huh.NewGroup(localSelect)).
			WithTheme(huh.ThemeCatppuccin()).
			Run()

		if err != nil {
			if err == huh.ErrUserAborted {
				fmt.Println(infoStyle.Render("Setup cancelled."))
				return
			}
			fmt.Println(errorStyle.Render("Error: " + err.Error()))
			return
		}

		// Set defaults based on selection
		defaultEndpoint := "http://localhost:1234"
		defaultModel := ""
		switch localProvider {
		case "lmstudio":
			defaultEndpoint = "http://localhost:1234"
			defaultModel = "your-model-name"
		case "ollama":
			defaultEndpoint = "http://localhost:11434"
			defaultModel = "llama3"
		case "other":
			defaultEndpoint = "http://localhost:8080"
			defaultModel = "model-name"
		}

		endpointInput := huh.NewInput().
			Title("LLM Endpoint URL").
			Description("The URL where your local LLM server is running").
			Placeholder(defaultEndpoint).
			Value(&endpoint)

		modelInput := huh.NewInput().
			Title("Model Name").
			Description("The name of the model to use").
			Placeholder(defaultModel).
			Value(&model)

		apiKeyInput := huh.NewInput().
			Title("API Key (optional)").
			Description("Leave empty if your server doesn't require authentication").
			Placeholder("(press Enter to skip)").
			Value(&apiKey)

		err = huh.NewForm(huh.NewGroup(endpointInput, modelInput, apiKeyInput)).
			WithTheme(huh.ThemeCatppuccin()).
			Run()

		if err != nil {
			if err == huh.ErrUserAborted {
				fmt.Println(infoStyle.Render("Setup cancelled."))
				return
			}
			fmt.Println(errorStyle.Render("Error: " + err.Error()))
			return
		}

		// Use defaults if empty
		if endpoint == "" {
			endpoint = defaultEndpoint
		}
		if model == "" {
			model = defaultModel
		}

	} else {
		// Azure OpenAI setup
		fmt.Println(infoStyle.Render("\nYou'll need your Azure OpenAI credentials from the Azure Portal."))
		fmt.Println(infoStyle.Render("Portal > Your OpenAI Resource > Keys and Endpoint\n"))

		endpointInput := huh.NewInput().
			Title("Azure OpenAI Endpoint").
			Description("e.g., https://your-resource.openai.azure.com").
			Placeholder("https://your-resource.openai.azure.com").
			Value(&endpoint)

		apiKeyInput := huh.NewInput().
			Title("Azure OpenAI API Key").
			Description("Your API key from the Azure Portal").
			Placeholder("your-api-key").
			EchoMode(huh.EchoModePassword).
			Value(&apiKey)

		modelInput := huh.NewInput().
			Title("Model Deployment Name").
			Description("e.g., gpt-4o, gpt-4o-mini").
			Placeholder("gpt-4o").
			Value(&model)

		apiVersionInput := huh.NewInput().
			Title("API Version (optional)").
			Description("Press Enter to use default: 2025-04-01-preview").
			Placeholder("2025-04-01-preview").
			Value(&apiVersion)

		err = huh.NewForm(huh.NewGroup(endpointInput, apiKeyInput, modelInput, apiVersionInput)).
			WithTheme(huh.ThemeCatppuccin()).
			Run()

		if err != nil {
			if err == huh.ErrUserAborted {
				fmt.Println(infoStyle.Render("Setup cancelled."))
				return
			}
			fmt.Println(errorStyle.Render("Error: " + err.Error()))
			return
		}

		// Validation
		if endpoint == "" || apiKey == "" || model == "" {
			fmt.Println(errorStyle.Render("Error: Endpoint, API Key, and Model are required for Azure OpenAI"))
			return
		}
	}

	// Step 3: Choose where to save
	shell, profiles := detectShell()
	fmt.Println(infoStyle.Render(fmt.Sprintf("\nDetected shell: %s", filepath.Base(shell))))

	var selectedProfile string
	var profileOptions []huh.Option[string]

	for _, p := range profiles {
		label := p.name
		if p.detected {
			label += " (detected)"
		}
		profileOptions = append(profileOptions, huh.NewOption(label, p.path))
	}

	profileSelect := huh.NewSelect[string]().
		Title("Where would you like to save the configuration?").
		Options(profileOptions...).
		Value(&selectedProfile)

	err = huh.NewForm(huh.NewGroup(profileSelect)).
		WithTheme(huh.ThemeCatppuccin()).
		Run()

	if err != nil {
		if err == huh.ErrUserAborted {
			fmt.Println(infoStyle.Render("Setup cancelled."))
			return
		}
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		return
	}

	// Generate config content
	var configContent string
	shellType := getShellType(selectedProfile)

	if shellType == ShellDotEnv {
		configContent = generateDotEnv(provider, endpoint, apiKey, model, apiVersion)
	} else {
		configContent = generateEnvExports(provider, endpoint, apiKey, model, apiVersion, shellType)
	}

	// Show preview
	previewBox := boxStyle.Render(fmt.Sprintf("📄 Configuration to be written to %s:\n%s", selectedProfile, configContent))
	fmt.Println(previewBox)

	// Confirm
	var proceed bool
	confirmSelect := huh.NewConfirm().
		Title("Write this configuration?").
		Affirmative("Yes, save it!").
		Negative("No, cancel").
		Value(&proceed)

	err = huh.NewForm(huh.NewGroup(confirmSelect)).
		WithTheme(huh.ThemeCatppuccin()).
		Run()

	if err != nil || !proceed {
		fmt.Println(infoStyle.Render("Setup cancelled."))
		return
	}

	// Ensure parent directory exists (for new profiles)
	parentDir := filepath.Dir(selectedProfile)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		fmt.Println(errorStyle.Render("Error creating directory: " + err.Error()))
		return
	}

	// Write to file
	if shellType == ShellDotEnv {
		// For .env, create or overwrite
		err = os.WriteFile(selectedProfile, []byte(configContent), 0600)
	} else {
		// For shell profiles, append or create
		f, err2 := os.OpenFile(selectedProfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err2 != nil {
			fmt.Println(errorStyle.Render("Error opening file: " + err2.Error()))
			return
		}
		defer f.Close()
		_, err = f.WriteString(configContent)
	}

	if err != nil {
		fmt.Println(errorStyle.Render("Error writing configuration: " + err.Error()))
		return
	}

	// Success message with shell-appropriate reload instructions
	var reloadCmd string
	switch shellType {
	case ShellPowerShell:
		reloadCmd = fmt.Sprintf(". %s", selectedProfile)
	case ShellFish:
		reloadCmd = fmt.Sprintf("source %s", selectedProfile)
	case ShellDotEnv:
		reloadCmd = "(restart capycut - .env is loaded automatically)"
	default:
		reloadCmd = fmt.Sprintf("source %s", selectedProfile)
	}

	successBox := boxStyle.Render(fmt.Sprintf(
		"✅ Configuration saved to %s!\n\n"+
			"To apply the changes, either:\n"+
			"  • Restart your terminal, or\n"+
			"  • Run: %s\n\n"+
			"Then run 'capycut' to start clipping videos!",
		selectedProfile,
		reloadCmd,
	))
	fmt.Println(successStyle.Render(successBox))
}

func main() {
	flag.Parse()

	if helpFlag {
		printHelp()
		os.Exit(0)
	}

	if versionFlag || shortVersionFlag {
		fmt.Printf("capycut %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built:  %s\n", date)
		fmt.Printf("  go:     %s\n", runtime.Version())
		fmt.Printf("  os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	// Run setup wizard if requested
	if setupFlag {
		runSetupWizard()
		os.Exit(0)
	}

	// Run self-update if requested
	if updateFlag {
		if err := runSelfUpdate(); err != nil {
			fmt.Println(errorStyle.Render("Update failed: " + err.Error()))
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Enable debug mode via flag
	if debugFlag {
		os.Setenv("CAPYCUT_DEBUG", "1")
	}

	// Load .env file if it exists (won't error if missing)
	_ = godotenv.Load()

	// Set provider from flag (overrides env var)
	if providerFlag != "" {
		os.Setenv("LLM_PROVIDER", providerFlag)
	}

	// Print header
	fmt.Println(titleStyle.Render(capybaraLogo))

	// Check for ffmpeg
	if err := video.CheckFFmpeg(); err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		os.Exit(1)
	}
	if err := video.CheckFFprobe(); err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		os.Exit(1)
	}

	// Check for LLM config
	if err := ai.CheckConfig(); err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		fmt.Println(infoStyle.Render(ai.GetAPIKeyHelp()))
		os.Exit(1)
	}

	// If file and prompt are provided via args, run non-interactive mode
	if fileFlag != "" && promptFlag != "" {
		runNonInteractive(fileFlag, promptFlag, outputFlag)
		return
	}

	// If only one of file/prompt is provided, show error
	if fileFlag != "" || promptFlag != "" {
		fmt.Println(errorStyle.Render("Error: Both --file and --prompt are required for non-interactive mode"))
		fmt.Println(infoStyle.Render("Run 'capycut --help' for usage information"))
		os.Exit(1)
	}

	// Main interactive loop
	for {
		if !runClipWorkflow() {
			break
		}
	}

	fmt.Println(subtitleStyle.Render("\n🦫 Thanks for using CapyCut! Bye bye!"))
}

func runNonInteractive(videoPath, clipDescription, customOutput string) {
	// Validate video file exists
	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		fmt.Println(errorStyle.Render("Error: Video file not found: " + videoPath))
		os.Exit(1)
	}

	// Get video info
	fmt.Println(infoStyle.Render("Reading video information..."))
	videoInfo, err := video.GetVideoInfo(videoPath)
	if err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		os.Exit(1)
	}

	// Display video info
	infoBox := boxStyle.Render(fmt.Sprintf(
		"📹 %s\n⏱  Duration: %s",
		videoInfo.Filename,
		video.FormatDuration(videoInfo.Duration),
	))
	fmt.Println(infoBox)

	// Parse with AI
	fmt.Println(infoStyle.Render("🦫 Parsing clip request..."))
	parser, err := ai.NewParser()
	if err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	clipReq, err := parser.ParseClipRequest(ctx, clipDescription, videoInfo.Duration)
	if err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		os.Exit(1)
	}

	// Calculate clip duration
	clipDuration, err := video.CalculateClipDuration(clipReq.StartTime, clipReq.EndTime)
	if err != nil {
		fmt.Println(errorStyle.Render("Error calculating duration: " + err.Error()))
		os.Exit(1)
	}

	// Determine output path
	outputPath := customOutput
	if outputPath == "" {
		outputPath = video.GenerateOutputPath(videoPath, clipReq.StartTime, clipReq.EndTime)
	}

	// Show summary
	summaryBox := boxStyle.Render(fmt.Sprintf(
		"📋 Clip Summary\n\n"+
			"Input:    %s\n"+
			"Start:    %s\n"+
			"End:      %s\n"+
			"Duration: %s\n"+
			"Output:   %s",
		filepath.Base(videoPath),
		clipReq.StartTime,
		clipReq.EndTime,
		video.FormatDuration(clipDuration),
		filepath.Base(outputPath),
	))
	fmt.Println(summaryBox)

	// Execute clip
	fmt.Println(infoStyle.Render("🦫 Clipping video..."))
	params := video.ClipParams{
		InputPath:  videoPath,
		StartTime:  clipReq.StartTime,
		EndTime:    clipReq.EndTime,
		OutputPath: outputPath,
	}

	if err := video.ClipVideo(params); err != nil {
		fmt.Println(errorStyle.Render("Error clipping video: " + err.Error()))
		os.Exit(1)
	}

	// Get output file info
	outputInfo, _ := os.Stat(outputPath)
	outputSize := "unknown"
	if outputInfo != nil {
		outputSize = formatFileSize(outputInfo.Size())
	}

	// Success!
	successBox := boxStyle.Render(fmt.Sprintf(
		"✅ Done!\n\n"+
			"Saved to: %s\n"+
			"Size: %s",
		outputPath,
		outputSize,
	))
	fmt.Println(successStyle.Render(successBox))
}

func runClipWorkflow() bool {
	// Step 1: Select video file
	var videoPath string
	startDir, _ := os.Getwd()

	filePicker := huh.NewFilePicker().
		Title("Select a video file").
		Description("Navigate and select a video to clip").
		Picking(true).
		CurrentDirectory(startDir).
		ShowHidden(false).
		ShowPermissions(false).
		ShowSize(true).
		Height(15).
		AllowedTypes([]string{".mp4", ".mkv", ".mov", ".avi", ".webm", ".flv", ".wmv", ".m4v"}).
		Value(&videoPath)

	err := huh.NewForm(huh.NewGroup(filePicker)).
		WithTheme(huh.ThemeCatppuccin()).
		Run()

	if err != nil {
		if err == huh.ErrUserAborted {
			return false
		}
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		return false
	}

	// Get video info
	var videoInfo *video.VideoInfo
	err = spinner.New().
		Title("Reading video information...").
		Action(func() {
			videoInfo, err = video.GetVideoInfo(videoPath)
		}).
		Run()

	if err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		return askToContinue()
	}

	// Display video info
	infoBox := boxStyle.Render(fmt.Sprintf(
		"📹 %s\n⏱  Duration: %s",
		videoInfo.Filename,
		video.FormatDuration(videoInfo.Duration),
	))
	fmt.Println(infoBox)

	// Step 2: Get clip description
	var clipDescription string
	descInput := huh.NewText().
		Title("🤖 What would you like to clip?").
		Description("Describe in natural language, e.g.:\n• \"from 3 minutes to 5 minutes 30 seconds\"\n• \"first 2 minutes\"\n• \"start at 1:23, end at 4:56\"\n• \"last 45 seconds\"").
		Placeholder("Type your clip description here...").
		CharLimit(500).
		Value(&clipDescription)

	err = huh.NewForm(huh.NewGroup(descInput)).
		WithTheme(huh.ThemeCatppuccin()).
		Run()

	if err != nil {
		if err == huh.ErrUserAborted {
			return askToContinue()
		}
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		return askToContinue()
	}

	// Step 3: Parse with AI
	var clipReq *ai.ClipRequest
	var parseErr error

	err = spinner.New().
		Title("🦫 Chomp chomp... understanding your request...").
		Action(func() {
			parser, err := ai.NewParser()
			if err != nil {
				parseErr = err
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			clipReq, parseErr = parser.ParseClipRequest(ctx, clipDescription, videoInfo.Duration)
		}).
		Run()

	if err != nil || parseErr != nil {
		if parseErr != nil {
			fmt.Println(errorStyle.Render("Error: " + parseErr.Error()))
		} else {
			fmt.Println(errorStyle.Render("Error: " + err.Error()))
		}
		return askToContinue()
	}

	// Calculate clip duration
	clipDuration, err := video.CalculateClipDuration(clipReq.StartTime, clipReq.EndTime)
	if err != nil {
		fmt.Println(errorStyle.Render("Error calculating duration: " + err.Error()))
		return askToContinue()
	}

	// Generate output path
	outputPath := video.GenerateOutputPath(videoPath, clipReq.StartTime, clipReq.EndTime)

	// Step 4: Confirm
	summaryBox := boxStyle.Render(fmt.Sprintf(
		"📋 Clip Summary\n\n"+
			"Input:    %s\n"+
			"Start:    %s\n"+
			"End:      %s\n"+
			"Duration: %s\n"+
			"Output:   %s",
		filepath.Base(videoPath),
		clipReq.StartTime,
		clipReq.EndTime,
		video.FormatDuration(clipDuration),
		filepath.Base(outputPath),
	))
	fmt.Println(summaryBox)

	var proceed bool
	confirmSelect := huh.NewConfirm().
		Title("Proceed with this clip?").
		Affirmative("Yes, cut it!").
		Negative("No, cancel").
		Value(&proceed)

	err = huh.NewForm(huh.NewGroup(confirmSelect)).
		WithTheme(huh.ThemeCatppuccin()).
		Run()

	if err != nil || !proceed {
		fmt.Println(infoStyle.Render("Clip cancelled."))
		return askToContinue()
	}

	// Step 5: Execute clip
	var clipErr error
	err = spinner.New().
		Title("🦫 Chomp chomp... clipping video...").
		Action(func() {
			params := video.ClipParams{
				InputPath:  videoPath,
				StartTime:  clipReq.StartTime,
				EndTime:    clipReq.EndTime,
				OutputPath: outputPath,
			}
			clipErr = video.ClipVideo(params)
		}).
		Run()

	if err != nil || clipErr != nil {
		if clipErr != nil {
			fmt.Println(errorStyle.Render("Error clipping video: " + clipErr.Error()))
		} else {
			fmt.Println(errorStyle.Render("Error: " + err.Error()))
		}
		return askToContinue()
	}

	// Get output file info
	outputInfo, _ := os.Stat(outputPath)
	outputSize := "unknown"
	if outputInfo != nil {
		outputSize = formatFileSize(outputInfo.Size())
	}

	// Success!
	successBox := boxStyle.Render(fmt.Sprintf(
		"✅ Done!\n\n"+
			"Saved to: %s\n"+
			"Size: %s",
		outputPath,
		outputSize,
	))
	fmt.Println(successStyle.Render(successBox))

	return askToContinue()
}

func askToContinue() bool {
	var choice string
	selectNext := huh.NewSelect[string]().
		Title("What next?").
		Options(
			huh.NewOption("Clip another video", "another"),
			huh.NewOption("Exit", "exit"),
		).
		Value(&choice)

	err := huh.NewForm(huh.NewGroup(selectNext)).
		WithTheme(huh.ThemeCatppuccin()).
		Run()

	if err != nil {
		return false
	}

	return choice == "another"
}

func formatFileSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}
