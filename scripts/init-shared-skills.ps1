#requires -Version 5.1
# Link this checkout to workspace-owned skills; never copy or overwrite skills.
[CmdletBinding()]
param([string]$SharedSkillsPath)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
if ($env:OS -ne 'Windows_NT') {
    throw 'This initializer creates Windows Junctions and must run on Windows.'
}

$repositoryRoot = Split-Path -Parent $PSScriptRoot
$agentsPath = Join-Path $repositoryRoot '.agents'
$linkPath = Join-Path $agentsPath 'skills'
$trackedSkills = @(& git -C $repositoryRoot ls-files -- .agents/skills)
if ($LASTEXITCODE -ne 0) { throw 'Could not inspect tracked skills.' }
if ($trackedSkills.Count -gt 0) {
    throw 'This checkout still tracks .agents/skills. Update to the shared-skills fix before initializing.'
}

if ([string]::IsNullOrWhiteSpace($SharedSkillsPath)) {
    $SharedSkillsPath = Join-Path (Split-Path -Parent $repositoryRoot) '.agents\skills'
}
$source = Get-Item -LiteralPath $SharedSkillsPath -Force -ErrorAction SilentlyContinue
if ($null -eq $source -or -not $source.PSIsContainer) {
    throw "Shared skills directory is missing: $SharedSkillsPath. Restore the workspace-owned skills first."
}
$sourcePath = $source.FullName.TrimEnd('\')
$repositoryPrefix = $repositoryRoot.TrimEnd('\') + '\'
if ($sourcePath.Equals($repositoryRoot, [StringComparison]::OrdinalIgnoreCase) -or
    $sourcePath.StartsWith($repositoryPrefix, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Shared skills must live outside this repository.'
}

$agents = Get-Item -LiteralPath $agentsPath -Force -ErrorAction SilentlyContinue
if ($null -ne $agents -and
    (-not $agents.PSIsContainer -or ($agents.Attributes -band [IO.FileAttributes]::ReparsePoint))) {
    throw '.agents must be an ordinary directory; its existing contents were preserved.'
}
if ($null -eq $agents) {
    New-Item -ItemType Directory -Path $agentsPath | Out-Null
}

$link = Get-Item -LiteralPath $linkPath -Force -ErrorAction SilentlyContinue
if ($null -ne $link) {
    if ($link.Attributes -band [IO.FileAttributes]::ReparsePoint) {
        $targets = @($link.Target)
        if ($link.LinkType -eq 'Junction' -and $targets.Count -eq 1) {
            $existingTarget = [IO.Path]::GetFullPath([string]$targets[0]).TrimEnd('\')
            if ($existingTarget.Equals($sourcePath, [StringComparison]::OrdinalIgnoreCase)) {
                Write-Output "Already linked: $linkPath -> $sourcePath"
                return
            }
        }
        throw "An existing link points elsewhere: $linkPath. It was preserved."
    }
    if (-not $link.PSIsContainer -or @(Get-ChildItem -LiteralPath $linkPath -Force).Count -gt 0) {
        throw "A nonempty directory or file exists at $linkPath. Back it up and reconcile it with shared skills first."
    }
    # Nonrecursive deletion fails if another process adds a file after the check.
    [IO.Directory]::Delete($linkPath)
}

New-Item -ItemType Junction -Path $linkPath -Target $sourcePath | Out-Null
Write-Output "Created Junction: $linkPath -> $sourcePath"
