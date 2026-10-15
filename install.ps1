$ErrorActionPreference = "Stop"

$Repo = "pablontiv/backscroll"
$Binary = "backscroll"

function Main {
    $arch = Get-Arch
    $version = Get-LatestVersion
    $installDir = Get-InstallDir
    Install-Binary -Version $version -Arch $arch -InstallDir $installDir
    Install-InputPresets -InstallDir $installDir
    Verify-Installation -InstallDir $installDir
}

function Get-Arch {
    switch ($env:PROCESSOR_ARCHITECTURE) {
        "AMD64" { return "amd64" }
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

    $semver = $Version -replace '^v',''
    $assetName = "${Binary}_${semver}_windows_${Arch}.zip"
    $url = "https://github.com/$Repo/releases/download/$Version/$assetName"
    $destPath = Join-Path $InstallDir "$Binary.exe"

    $tmpRoot = Join-Path $env:TEMP "backscroll-install-$([System.Guid]::NewGuid().ToString('N'))"
    $zipPath = Join-Path $tmpRoot "asset.zip"
    $extractDir = Join-Path $tmpRoot "extract"

    Write-Host "Downloading $assetName..."
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    New-Item -ItemType Directory -Path $extractDir -Force | Out-Null

    try {
        Invoke-WebRequest -Uri $url -OutFile $zipPath -UseBasicParsing
        Expand-Archive -Path $zipPath -DestinationPath $extractDir -Force

        $extractedBinary = Join-Path $extractDir "$Binary.exe"
        if (-not (Test-Path $extractedBinary)) {
            throw "Archive $assetName did not contain $Binary.exe"
        }

        Move-Item -Path $extractedBinary -Destination $destPath -Force
    }
    finally {
        if (Test-Path $tmpRoot) {
            Remove-Item -Path $tmpRoot -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

function Get-ConfigDir {
    if ($env:BACKSCROLL_CONFIG_DIR) {
        return $env:BACKSCROLL_CONFIG_DIR
    }

    $homeDir = $HOME
    if (-not $homeDir) {
        $homeDir = [Environment]::GetFolderPath("UserProfile")
    }
    if ($homeDir) {
        return (Join-Path $homeDir ".config")
    }

    throw "Could not determine user home directory. Set BACKSCROLL_CONFIG_DIR."
}

function Install-InputPresets {
    param($InstallDir)

    $exe = Join-Path $InstallDir "$Binary.exe"
    $arguments = @("presets", "install", "--config-home", (Get-ConfigDir))
    if ($env:BACKSCROLL_FORCE_INPUTS -eq "1") {
        $arguments += "--overwrite"
    }
    Write-Host "Installing embedded input presets"
    & $exe @arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Input preset installation failed with exit code $LASTEXITCODE"
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
