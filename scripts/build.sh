#!/bin/bash
# Build script — builds production .app bundle + .dmg
set -euo pipefail

APP_NAME="ORCH"
VERSION="${1:-1.0.0}"
BUILD_DIR="build/bin"
APP_PATH="${BUILD_DIR}/${APP_NAME}.app"
DMG_PATH="${BUILD_DIR}/${APP_NAME}-${VERSION}.dmg"

echo "📦 Building ${APP_NAME} for macOS (universal)..."
wails build -platform darwin/universal -clean

if [ ! -d "${APP_PATH}" ]; then
  echo "❌ App bundle not found at ${APP_PATH}"
  exit 1
fi

echo "💿 Packaging DMG..."
rm -f "${DMG_PATH}"
hdiutil create -volname "${APP_NAME}" -srcfolder "${APP_PATH}" -ov -format UDZO "${DMG_PATH}" >/dev/null

echo "✅ Build complete"
echo "   App: ${APP_PATH}"
echo "   DMG: ${DMG_PATH}"
