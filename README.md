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
go install github.com/harmonyvt/capycut@latest
```

Or clone and build:

```bash
git clone https://github.com/harmonyvt/capycut.git
cd capycut
go build -o capycut .
```

## Configuration

CapyCut uses Azure OpenAI for natural language parsing. Set up your credentials:

### Option 1: Environment Variables

```bash
export AZURE_OPENAI_ENDPOINT="https://your-resource.openai.azure.com"
export AZURE_OPENAI_API_KEY="your-api-key"
export AZURE_OPENAI_MODEL="gpt-4o"
export AZURE_OPENAI_API_VERSION="2024-08-01-preview"  # optional
```

### Option 2: .env File

Copy the example and fill in your values:

```bash
cp .env.example .env
# Edit .env with your credentials
```

## Usage

Simply run:

```bash
capycut
```

Then:
1. Select a video file
2. Describe what you want to clip in natural language:
   - "from 3 minutes to 5 minutes 30 seconds"
   - "first 2 minutes"
   - "start at 1:23, end at 4:56"
   - "last 45 seconds"
3. Confirm and clip!

### Version

```bash
capycut --version
```

## Development

```bash
# Clone the repo
git clone https://github.com/harmonyvt/capycut.git
cd capycut

# Copy and configure environment
cp .env.example .env
# Edit .env with your Azure OpenAI credentials

# Run commands
make help          # Show all commands
make run           # Build and run
make test          # Run tests
make release-dry   # Test release build locally
```

## License

MIT
