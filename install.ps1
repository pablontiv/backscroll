$ErrorActionPreference = "Stop"

$Repo = "pablontiv/backscroll"
$Binary = "backscroll"
$InputPresets = @("claude.inputs.toml", "pi.inputs.toml")

function Main {
    $arch = Get-Arch
    $version = Get-LatestVersion
    $installDir = Get-InstallDir
    Install-Binary -Version $version -Arch $arch -InstallDir $installDir
    Install-InputPresets -Version $version
    Verify-Installation -InstallDir $installDir
}

function Get-Arch {
    switch ($env:PROCESSOR_ARCHITECTURE) {
        "AMD64" { return "x86_64" }
        default { throw "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE. Only AMD64 is supported." }
    }
}

function Get-LatestVersion {
    Write-Host "Fetching latest version..."
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
    $version = $release.tag_name
    if (-not $version) {
        throw "Could not determine latest version. Check https://github.com/$Repo/releases"
    }
    Write-Host "Latest version: $version"
    return $version
}

function Get-InstallDir {
    if ($env:BACKSCROLL_INSTALL_DIR) {
        return $env:BACKSCROLL_INSTALL_DIR
    }

    $baseDir = Join-Path $env:LOCALAPPDATA "backscroll"
    $dir = Join-Path $baseDir "bin"

    if (-not (Test-Path $dir)) {
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
    }

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$dir*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$dir", "User")
        $env:Path = "$env:Path;$dir"
        Write-Host "Added $dir to user PATH"
    }

    return $dir
}

function Install-Binary {
    param($Version, $Arch, $InstallDir)

    $assetName = "${Binary}-windows-${Arch}.exe"
    $url = "https://github.com/$Repo/releases/download/$Version/$assetName"
    $destPath = Join-Path $InstallDir "$Binary.exe"

    Write-Host "Downloading $assetName..."
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Invoke-WebRequest -Uri $url -OutFile $destPath -UseBasicParsing
}

function Get-ConfigDir {
    if ($env:BACKSCROLL_CONFIG_DIR) {
        return $env:BACKSCROLL_CONFIG_DIR
    }

    if ($env:APPDATA) {
        return $env:APPDATA
    }

    $appData = [Environment]::GetFolderPath("ApplicationData")
    if ($appData) {
        return $appData
    }

    throw "Could not determine config directory. Set BACKSCROLL_CONFIG_DIR."
}

function Get-LocalInputsDir {
    if ($env:BACKSCROLL_INPUTS_SOURCE_DIR) {
        return $env:BACKSCROLL_INPUTS_SOURCE_DIR
    }

    if ($PSScriptRoot) {
        return (Join-Path $PSScriptRoot "inputs")
    }

    return $null
}

function Install-InputPresets {
    param($Version, $SourceDir)

    $configDir = Get-ConfigDir
    $backscrollDir = Join-Path $configDir "backscroll"
    $inputsDir = Join-Path $backscrollDir "inputs"
    if (-not $SourceDir) {
        $SourceDir = Get-LocalInputsDir
    }

    New-Item -ItemType Directory -Path $inputsDir -Force | Out-Null
    Write-Host "Installing input presets to $inputsDir"

    foreach ($preset in $InputPresets) {
        $destPath = Join-Path $inputsDir $preset
        if ((Test-Path $destPath) -and ($env:BACKSCROLL_FORCE_INPUTS -ne "1")) {
            Write-Host "$destPath exists, skipping"
            continue
        }

        $sourcePath = $null
        if ($SourceDir) {
            $candidate = Join-Path $SourceDir $preset
            if (Test-Path $candidate) {
                $sourcePath = $candidate
            }
        }

        if ($sourcePath) {
            Copy-Item -Path $sourcePath -Destination $destPath -Force
        }
        else {
            if (-not $Version) {
                $Version = "main"
            }
            $url = "https://raw.githubusercontent.com/$Repo/$Version/inputs/$preset"
            $tmpPath = "$destPath.tmp"
            Invoke-WebRequest -Uri $url -OutFile $tmpPath -UseBasicParsing
            Move-Item -Path $tmpPath -Destination $destPath -Force
        }
        Write-Host "Installed $preset"
    }
}

function Verify-Installation {
    param($InstallDir)

    $exe = Join-Path $InstallDir "$Binary.exe"
    if (Test-Path $exe) {
        $ver = & $exe --version 2>&1
        Write-Host "Installed $ver to $exe"
    }
    else {
        throw "Installation failed: $exe not found"
    }
}

Main
