# yhat-agent

Installable Go CLI for the YHat knowledge capture agent and skill.

## What it does

`yhat-agent` is a self-contained binary that installs the YHat OpenCode agent and skill assets into your local OpenCode configuration directory. It tracks only its own files and can safely update from GitHub Releases.

## Installation

```bash
go install github.com/ArcKelMiranda/yhat-agent/cmd/yhat-agent@latest
```

Or download pre-built binaries from the [Releases page](https://github.com/ArcKelMiranda/yhat-agent/releases).

## Usage

```bash
yhat-agent install   # Install embedded agent and skill assets
yhat-agent status    # Report installation state without making changes
yhat-agent update    # Download and verify latest release from GitHub
yhat-agent uninstall --yes   # Remove managed files (requires confirmation)
```

## Source layout

```
yhat-agent/
├── cmd/yhat-agent/          # CLI entry point
├── assets/                  # Embedded agent and skill assets
│   ├── agents/              # OpenCode agent definitions
│   └── skills/              # OpenCode skill definitions
├── *.go                     # Core logic: install, status, update, uninstall, manifest, paths
└── *_test.go                # Test suite
```

## License

MIT
