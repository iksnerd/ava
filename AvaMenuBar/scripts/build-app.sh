#!/bin/bash
# Builds AvaMenuBar (release) and packages it as a real double-clickable
# .app bundle at .build/Ava.app — then installs it to /Applications.
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_ROOT="$(cd "$DIR/.." && pwd)"
cd "$DIR"

APP_NAME="Ava"
BUNDLE_ID="xyz.iksnerd.ava"
BIN_NAME="AvaMenuBar"
APP_DIR="$DIR/.build/$APP_NAME.app"
INSTALLED_APP="/Applications/$APP_NAME.app"
DMG_PATH="$DIR/.build/$APP_NAME.dmg"

echo "🔨 Building release binary..."
swift build -c release
RELEASE_BIN="$(swift build -c release --show-bin-path)/$BIN_NAME"

echo "🔨 Building ava (bundled for Dictate — whisper.cpp, no extra setup)..."
make -C "$REPO_ROOT" build

echo "📦 Packaging $APP_NAME.app..."
rm -rf "$APP_DIR"
mkdir -p "$APP_DIR/Contents/MacOS" "$APP_DIR/Contents/Resources"
cp "$RELEASE_BIN" "$APP_DIR/Contents/MacOS/$BIN_NAME"
cp "$DIR/Resources/AppIcon.icns" "$APP_DIR/Contents/Resources/AppIcon.icns"

# scripts/ + the ava binary get bundled so the app works from any
# checkout (Paths.swift resolves both relative to this Resources/ dir at
# runtime, falling back to the dev-checkout path only when unbundled).
# .venv/__pycache__ are excluded: per-machine and uv-managed — voice_hooks/
# syncs its own fresh on first use, same as `make setup-voice-hooks` today.
# mlx-engine/ is deliberately NOT bundled: its venv alone is ~1.2GB, and the
# Kokoro model downloads on first use anyway. The app runs the checkout's
# engine instead (engine-root, below), or the one `ava setup` installed, and
# speak.sh falls back to macOS `say` when neither exists.
rsync -a --exclude '.venv' --exclude '__pycache__' --exclude '*.pyc' \
    "$REPO_ROOT/scripts/" "$APP_DIR/Contents/Resources/scripts/"
cp "$REPO_ROOT/bin/ava" "$APP_DIR/Contents/Resources/ava"
# mlx-engine/ is not bundled (see above), so record where it lives: the
# bundled mlx-engine-server.sh reads this to find the checkout's engine, which
# is what the menu bar's Start button and speech auto-start run. Without it
# the app could only use a server that something else had already started.
echo "$REPO_ROOT" > "$APP_DIR/Contents/Resources/engine-root"

cat > "$APP_DIR/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>
    <string>$APP_NAME</string>
    <key>CFBundleDisplayName</key>
    <string>$APP_NAME</string>
    <key>CFBundleIdentifier</key>
    <string>$BUNDLE_ID</string>
    <key>CFBundleExecutable</key>
    <string>$BIN_NAME</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>1.0</string>
    <key>CFBundleVersion</key>
    <string>1</string>
    <key>LSUIElement</key>
    <true/>
    <key>LSMinimumSystemVersion</key>
    <string>13.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSServices</key>
    <array>
        <dict>
            <key>NSMenuItem</key>
            <dict>
                <key>default</key>
                <string>Read Aloud with Ava</string>
            </dict>
            <key>NSMessage</key>
            <string>readAloud</string>
            <key>NSPortName</key>
            <string>$APP_NAME</string>
            <key>NSSendTypes</key>
            <array>
                <string>public.utf8-plain-text</string>
                <string>NSStringPboardType</string>
            </array>
        </dict>
        <dict>
            <key>NSMenuItem</key>
            <dict>
                <key>default</key>
                <string>Toggle Ava Mute</string>
            </dict>
            <key>NSMessage</key>
            <string>toggleMute</string>
            <key>NSPortName</key>
            <string>$APP_NAME</string>
        </dict>
    </array>
</dict>
</plist>
PLIST

echo "✍️  Ad-hoc code signing..."
codesign --force --deep -s - "$APP_DIR"

echo "🚚 Installing to $INSTALLED_APP..."
rm -rf "$INSTALLED_APP"
cp -R "$APP_DIR" "$INSTALLED_APP"
codesign --force --deep -s - "$INSTALLED_APP"

echo "💿 Creating $APP_NAME.dmg..."
rm -f "$DMG_PATH"
# The .dmg is for another Mac, where this machine's checkout path means
# nothing and would only leak a home directory, so pack a copy without
# engine-root. Removing a file breaks the seal, hence the re-sign.
DMG_STAGE="$(mktemp -d)"
trap 'rm -rf "$DMG_STAGE"' EXIT
cp -R "$APP_DIR" "$DMG_STAGE/"
rm -f "$DMG_STAGE/$APP_NAME.app/Contents/Resources/engine-root"
codesign --force --deep -s - "$DMG_STAGE/$APP_NAME.app"
hdiutil create -volname "$APP_NAME" -srcfolder "$DMG_STAGE/$APP_NAME.app" -ov -format UDZO "$DMG_PATH" >/dev/null

echo "✅ Installed. Launch with: open -a \"$APP_NAME\""
echo "   Note: after (re)installing, 'Read Aloud with Ava' can take a"
echo "   minute to appear in other apps' right-click Services menu — relaunching"
echo "   $APP_NAME (already done via NSUpdateDynamicServices on launch) usually"
echo "   surfaces it immediately; if not, log out and back in."
echo ""
echo "📀 $APP_NAME.dmg: $DMG_PATH"
echo "   No Developer ID — this is ad-hoc signed (codesign -s -), not notarized."
echo "   On another Mac, opening it shows Gatekeeper's standard \"Apple could"
echo "   not verify ... is free of malware\" warning (verified directly: a"
echo "   quarantined copy of this exact build gets blocked this way, not the"
echo "   harsher \"is damaged\" message). The recipient can either:"
echo "     - System Settings → Privacy & Security → scroll down → \"Open"
echo "       Anyway\" next to the blocked-app notice, confirm once more, or"
echo "     - xattr -cr \"/Applications/$APP_NAME.app\" (skips the dialog"
echo "       entirely — verified this actually launches clean afterward)."
