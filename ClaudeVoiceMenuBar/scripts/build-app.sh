#!/bin/bash
# Builds ClaudeVoiceMenuBar (release) and packages it as a real double-clickable
# .app bundle at .build/Claude Voice.app — then installs it to /Applications.
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$DIR"

APP_NAME="Claude Voice"
BUNDLE_ID="com.local-whisper.claudevoice"
BIN_NAME="ClaudeVoiceMenuBar"
APP_DIR="$DIR/.build/$APP_NAME.app"
INSTALLED_APP="/Applications/$APP_NAME.app"

echo "🔨 Building release binary..."
swift build -c release
RELEASE_BIN="$(swift build -c release --show-bin-path)/$BIN_NAME"

echo "📦 Packaging $APP_NAME.app..."
rm -rf "$APP_DIR"
mkdir -p "$APP_DIR/Contents/MacOS" "$APP_DIR/Contents/Resources"
cp "$RELEASE_BIN" "$APP_DIR/Contents/MacOS/$BIN_NAME"
cp "$DIR/Resources/AppIcon.icns" "$APP_DIR/Contents/Resources/AppIcon.icns"

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
                <string>Read Aloud with Claude Voice</string>
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
                <string>Toggle Claude Voice Mute</string>
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

echo "✅ Installed. Launch with: open -a \"$APP_NAME\""
echo "   Note: after (re)installing, 'Read Aloud with Claude Voice' can take a"
echo "   minute to appear in other apps' right-click Services menu — relaunching"
echo "   $APP_NAME (already done via NSUpdateDynamicServices on launch) usually"
echo "   surfaces it immediately; if not, log out and back in."
