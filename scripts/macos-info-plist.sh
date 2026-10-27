#!/usr/bin/env bash
# Writes the Info.plist for "Valheim Mod Pusher.app" (GoReleaser before-hook).
# Usage: scripts/macos-info-plist.sh <version> <output file>
set -euo pipefail
version=$1
out=$2
mkdir -p "$(dirname "$out")"
cat >"$out" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>Valheim Mod Pusher</string>
	<key>CFBundleDisplayName</key>
	<string>Valheim Mod Pusher</string>
	<key>CFBundleExecutable</key>
	<string>valheim-mod-pusher</string>
	<key>CFBundleIdentifier</key>
	<string>io.github.rgeraskin.valheimmodpusher</string>
	<key>CFBundleIconFile</key>
	<string>Icon</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>${version}</string>
	<key>CFBundleVersion</key>
	<string>${version}</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>LSMinimumSystemVersion</key>
	<string>13.0</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>NSSupportsAutomaticGraphicsSwitching</key>
	<true/>
</dict>
</plist>
PLIST
