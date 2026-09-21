#!/bin/bash

# Installs BlackHole (virtual audio loopback driver) so system/app audio
# (e.g. a Google Meet call) can be captured as an input device by
# voxtral/realtime.py, instead of only the physical microphone.
#
# BlackHole installs a system audio driver - macOS requires a reboot before
# it shows up as an input/output device. Creating a Multi-Output Device (so
# you can still hear the call while it's captured) has to be done by hand in
# Audio MIDI Setup - that part can't be scripted.

echo "🎧 BlackHole Setup (system audio capture)"
echo ""

if ! command -v brew &> /dev/null; then
    echo "❌ Homebrew not found. Install it first: https://brew.sh"
    exit 1
fi

if brew list --cask blackhole-2ch &> /dev/null; then
    echo "✅ BlackHole already installed"
else
    echo "⬇️  Installing BlackHole 2ch (you may be prompted for your password)..."
    brew install --cask blackhole-2ch
    if [ $? -ne 0 ]; then
        echo "❌ Failed to install BlackHole"
        exit 1
    fi
    echo "⚠️  Reboot required before BlackHole appears as an audio device."
fi

if ! command -v SwitchAudioSource &> /dev/null; then
    echo "⬇️  Installing switchaudio-osx (CLI audio device switching)..."
    brew install switchaudio-osx
fi

echo ""
echo "One-time manual setup (Audio MIDI Setup can't be scripted):"
echo "  1. Open /Applications/Utilities/Audio MIDI Setup.app"
echo "  2. Click '+' (bottom left) -> Create Multi-Output Device"
echo "  3. Check both your normal output (e.g. MacBook Pro Speakers) and BlackHole 2ch"
echo "  4. System Settings -> Sound -> Output -> select that Multi-Output Device"
echo "     (this is what lets you still HEAR the call while BlackHole captures it)"
echo "  5. Leave Google Meet's audio output on System Default so it plays through it"
echo ""
echo "Find BlackHole's device index/name with:"
echo "  make build-voice-monitor && bin/voice-monitor devices"
echo ""
echo "Then watch a call live at http://localhost:8766 with:"
echo "  bin/voice-monitor --device BlackHole"
exit 0
