# Textbridge

A Tauri 2 desktop application.

## Setup

```bash
# Clone and initialise
git clone git@github.com-personal:jamesdowzard/tauri-app-template.git textbridge
cd textbridge
rm -rf .git && git init
./init.sh
```

## Development

```bash
# Run in development mode
cargo tauri dev

# Build, sign, and install
./build.sh
```

## Stack

- **Frontend**: Vite + TypeScript
- **Backend**: Tauri 2 (Rust)
- **Signing**: Developer ID (pre-configured)
- **Linting**: Biome

## Features

- Pre-configured code signing (permissions persist)
- macOS entitlements for accessibility/automation
- Minimal starter template
- Harness-ready for parallel agent development
