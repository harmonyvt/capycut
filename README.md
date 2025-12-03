# CapyCut

<!-- LATEST_RELEASE_START -->
[![Latest Release](https://img.shields.io/github/v/release/harmonyvt/capycut?label=latest)](https://github.com/harmonyvt/capycut/releases/latest)
<!-- LATEST_RELEASE_END -->

AI-powered video clipper CLI. Describe what you want to clip in natural language, and CapyCut will do the rest.

## Installation

### Quick Install (Recommended)

**macOS / Linux:**
```bash
curl -fsSL https://raw.githubusercontent.com/harmonyvt/capycut/master/install.sh | sh
```

Or with wget:
```bash
wget -qO- https://raw.githubusercontent.com/harmonyvt/capycut/master/install.sh | sh
```

**Windows (PowerShell):**
```powershell
irm https://raw.githubusercontent.com/harmonyvt/capycut/master/install.ps1 | iex
```

### Prerequisites

**FFmpeg is required.** Install it for your platform:

```bash
# macOS
brew install ffmpeg

# Ubuntu/Debian
sudo apt install ffmpeg

# Fedora
sudo dnf install ffmpeg

# Arch
sudo pacman -S ffmpeg

# Windows (winget)
winget install ffmpeg

# Windows (Chocolatey)
choco install ffmpeg
```

### Updating

CapyCut can update itself! Simply run:

```bash
capycut --update
```

### Manual Installation

<details>
<summary>macOS</summary>

```bash
# Apple Silicon (M1/M2/M3)
curl -L https://github.com/harmonyvt/capycut/releases/latest/download/capycut_darwin_arm64.tar.gz | tar xz
sudo mv capycut /usr/local/bin/

# Intel Macs
curl -L https://github.com/harmonyvt/capycut/releases/latest/download/capycut_darwin_amd64.tar.gz | tar xz
sudo mv capycut /usr/local/bin/
```
</details>

<details>
<summary>Linux</summary>

```bash
# x86_64 / amd64
curl -L https://github.com/harmonyvt/capycut/releases/latest/download/capycut_linux_amd64.tar.gz | tar xz
sudo mv capycut /usr/local/bin/

# ARM64 (e.g., Raspberry Pi)
curl -L https://github.com/harmonyvt/capycut/releases/latest/download/capycut_linux_arm64.tar.gz | tar xz
sudo mv capycut /usr/local/bin/
```
</details>

<details>
<summary>Windows</summary>

1. Download `capycut_windows_amd64.zip` from [GitHub Releases](https://github.com/harmonyvt/capycut/releases/latest)
2. Extract the zip file
3. Move `capycut.exe` to a directory in your PATH

Or via PowerShell:
```powershell
Invoke-WebRequest -Uri "https://github.com/harmonyvt/capycut/releases/latest/download/capycut_windows_amd64.zip" -OutFile "capycut.zip"
Expand-Archive -Path "capycut.zip" -DestinationPath "."
Move-Item -Path "capycut.exe" -Destination "$env:LOCALAPPDATA\Programs\capycut\"
```
</details>

### Build from Source

Requires [Go 1.21+](https://go.dev/dl/).

```bash
git clone https://github.com/harmonyvt/capycut.git
cd capycut
go build -o capycut .

# On Windows, the output will be capycut.exe
```

## Configuration

CapyCut uses an LLM to parse natural language commands. Choose either a **local LLM** (free, private) or **Azure OpenAI**.

### Option 1: Local LLM (Recommended - FREE!)

Run models locally with [LM Studio](https://lmstudio.ai) or [Ollama](https://ollama.ai). No API keys needed!

#### LM Studio

1. Download [LM Studio](https://lmstudio.ai)
2. Load a model (Llama 3, Mistral, Qwen, Phi, etc.)
3. Start the local server (default: `http://localhost:1234`)
4. Configure:

```bash
export LLM_ENDPOINT="http://localhost:1234"
```

#### Ollama

1. Install [Ollama](https://ollama.ai)
2. Pull and run a model: `ollama run llama3.2`
3. Configure:

```bash
export LLM_ENDPOINT="http://localhost:11434"
export LLM_MODEL="llama3.2"
```

### Option 2: Azure OpenAI

```bash
export AZURE_OPENAI_ENDPOINT="https://your-resource.cognitiveservices.azure.com"
export AZURE_OPENAI_API_KEY="your-api-key"
export AZURE_OPENAI_MODEL="gpt-4o"
```

### Using a .env File

```bash
cp .env.example .env
# Edit .env with your settings
```

## Usage

```bash
capycut
```

Then:
1. Select a video file
2. Describe what you want to clip:
   - "from 3 minutes to 5 minutes 30 seconds"
   - "first 2 minutes"
   - "start at 1:23, end at 4:56"
   - "last 45 seconds"
3. Confirm and clip!

### Debug Mode

```bash
capycut --debug
```

Shows detailed info about API calls for troubleshooting.

## Development

```bash
git clone https://github.com/harmonyvt/capycut.git
cd capycut
cp .env.example .env

make help          # Show all commands
make run           # Build and run
make test          # Run tests
make release-dry   # Test release build locally
```

## License

MIT
