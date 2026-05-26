#!/usr/bin/env bash
# ---
# name: install
# description: Install mutter and all bundled binaries for macOS and Linux
# usage: curl -o- https://raw.githubusercontent.com/sidisinsane/mutter/main/install.sh | bash
# exits:
#   0: success
#   1: unsupported OS, unsupported architecture, download failure, or extraction failure
# ---

set -eo pipefail

GITHUB_USER="sidisinsane"
GITHUB_REPO="mutter"
BINARY_NAME="mutter"

# Binaries bundled in the release archive.
# Uncomment each entry as the corresponding binary is added to .goreleaser.yaml.
BINARIES=(
  # "mutter"
  "mutter-daemon"
  # "mutter-web"
  # "mutter-launcher"
)

# 1. Detect OS
OS_TYPE=$(uname -s)
case "$OS_TYPE" in
    Darwin*) PLATFORM="Darwin" ;;
    Linux*)  PLATFORM="Linux"  ;;
    *)       echo "Error: Unsupported OS: $OS_TYPE"; exit 1 ;;
esac

# 2. Detect architecture
ARCH_TYPE=$(uname -m)
case "$ARCH_TYPE" in
    x86_64)        ARCH="x86_64" ;;
    arm64|aarch64) ARCH="arm64"  ;;
    *)             echo "Error: Unsupported architecture: $ARCH_TYPE"; exit 1 ;;
esac

# 3. Detect shell profile
if [[ "$SHELL" == */zsh ]]; then
    SHELL_CONFIG="$HOME/.zshrc"
elif [[ "$SHELL" == */bash ]]; then
    SHELL_CONFIG="$HOME/.bashrc"
else
    SHELL_CONFIG="$HOME/.profile"
fi

# 4. Construct filename and download URL
FILENAME="${GITHUB_REPO}_${PLATFORM}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${GITHUB_USER}/${GITHUB_REPO}/releases/latest/download/${FILENAME}"

# 5. Create installation directory
INSTALL_DIR="$HOME/.$BINARY_NAME"
mkdir -p "$INSTALL_DIR"

# 6. Download and extract
echo "Downloading $BINARY_NAME for $PLATFORM ($ARCH)..."
curl -fsSL "$DOWNLOAD_URL" | tar -xz -C "$INSTALL_DIR"

# 7. Make all binaries executable and clear macOS quarantine
for bin in "${BINARIES[@]}"; do
    if [ -f "$INSTALL_DIR/$bin" ]; then
        chmod +x "$INSTALL_DIR/$bin"
        if [ "$PLATFORM" = "Darwin" ]; then
            xattr -d com.apple.quarantine "$INSTALL_DIR/$bin" 2>/dev/null || true
        fi
    fi
done

# 8. Add installation directory to PATH (once, regardless of binary count)
if ! grep -q "$INSTALL_DIR" "$SHELL_CONFIG" 2>/dev/null; then
    echo "Adding $INSTALL_DIR to PATH in $SHELL_CONFIG"
    echo "" >> "$SHELL_CONFIG"
    echo "# $BINARY_NAME" >> "$SHELL_CONFIG"
    echo "export PATH=\"\$PATH:$INSTALL_DIR\"" >> "$SHELL_CONFIG"
    echo "Done! Run 'source $SHELL_CONFIG' or restart your terminal to use $BINARY_NAME."
else
    echo "$BINARY_NAME is already in your PATH."
fi

echo "Installation complete!"
