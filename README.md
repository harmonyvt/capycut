# CapyCut

AI-powered video clipper CLI. Describe what you want to clip in natural language, and CapyCut will do the rest.

## Installation

### Prerequisites

**FFmpeg is required.** Install it for your platform:

- **macOS**: `brew install ffmpeg`
- **Ubuntu/Debian**: `sudo apt install ffmpeg`
- **Fedora**: `sudo dnf install ffmpeg`
- **Arch**: `sudo pacman -S ffmpeg`
- **Windows**: `winget install ffmpeg` or `choco install ffmpeg`

### Download Binary

Download the latest release from [GitHub Releases](https://github.com/harmonyvt/capycut/releases).

#### macOS / Linux

```bash
# Download (replace VERSION and ARCH as needed)
curl -L https://github.com/harmonyvt/capycut/releases/latest/download/capycut_VERSION_darwin_arm64.tar.gz | tar xz

# Move to PATH
sudo mv capycut /usr/local/bin/
```

#### Windows

Download the `.zip` from releases, extract, and add to your PATH.

### Build from Source

```bash
git clone https://github.com/harmonyvt/capycut.git
cd capycut
go build -o capycut .
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
