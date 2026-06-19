# claude-unstuck installer for Windows.
# Usage: irm https://raw.githubusercontent.com/jas0xf/claude-unstuck/main/install.ps1 | iex
$ErrorActionPreference = "Stop"

$repo = "jas0xf/claude-unstuck"
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq "Arm64") { "arm64" } else { "amd64" }

$release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
$tag = $release.tag_name
$ver = $tag.TrimStart("v")
$url = "https://github.com/$repo/releases/download/$tag/claude-unstuck_${ver}_windows_${arch}.zip"

$dest = Join-Path $env:LOCALAPPDATA "claude-unstuck"
New-Item -ItemType Directory -Force -Path $dest | Out-Null
$zip = Join-Path $env:TEMP "claude-unstuck.zip"

Write-Host "Downloading claude-unstuck $tag (windows/$arch)..."
Invoke-WebRequest -Uri $url -OutFile $zip
Expand-Archive -Path $zip -DestinationPath $dest -Force
Remove-Item $zip

# Add to user PATH if missing
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$dest*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$dest", "User")
    Write-Host "Added $dest to your user PATH (restart your terminal to pick it up)."
}

& (Join-Path $dest "claude-unstuck.exe") version
Write-Host "Try: claude-unstuck doctor"
Write-Host "Fix every app: run 'claude-unstuck on' in an Administrator PowerShell."
