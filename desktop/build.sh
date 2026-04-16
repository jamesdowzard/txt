#!/bin/bash
# Build and sign the Tauri app with Developer ID
set -e

cd "$(dirname "$0")"

# Read app name from tauri.conf.json
APP_NAME=$(grep -o '"productName": "[^"]*"' src-tauri/tauri.conf.json | cut -d'"' -f4)
if [ -z "$APP_NAME" ]; then
    echo "Error: Could not read productName from tauri.conf.json"
    exit 1
fi

APP_PATH="src-tauri/target/release/bundle/macos/${APP_NAME}.app"
ENTITLEMENTS="src-tauri/entitlements.plist"
IDENTITY="Developer ID Application: James Dowzard (G54DLMPV94)"

echo "Building ${APP_NAME}..."
cargo tauri build 2>&1 | grep -v "failed to remove extra attributes" || true

if [ ! -d "$APP_PATH" ]; then
    echo "Error: Build failed - app bundle not found at $APP_PATH"
    exit 1
fi

echo "Clearing extended attributes..."
find "$APP_PATH" -exec xattr -c {} \; 2>/dev/null || true

echo "Signing binary..."
codesign --force --options runtime --entitlements "$ENTITLEMENTS" --sign "$IDENTITY" "$APP_PATH/Contents/MacOS/"* 2>&1

echo "Signing bundle..."
codesign --force --options runtime --entitlements "$ENTITLEMENTS" --sign "$IDENTITY" "$APP_PATH"

echo "Installing to /Applications..."
rm -rf "/Applications/${APP_NAME}.app"
cp -R "$APP_PATH" /Applications/

echo ""
echo "Done. Installed: /Applications/${APP_NAME}.app"
echo ""
echo "Signature:"
codesign -dv "/Applications/${APP_NAME}.app" 2>&1 | grep -E "(Authority|TeamIdentifier)"
