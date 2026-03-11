$ErrorActionPreference = "Stop"

$Repo = "pablontiv/backscroll"
$Binary = "backscroll"

function Main {
    $arch = Get-Arch
    $version = Get-LatestVersion
    $installDir = Get-InstallDir
    Install-Binary -Version $version -Arch $arch -InstallDir $installDir
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
