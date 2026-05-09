# Pester tests for install.ps1
# Run with: Invoke-Pester -Path tests/test-install.ps1
# Static only: Invoke-Pester -Path tests/test-install.ps1 -Tag "Static"
# Runtime only: Invoke-Pester -Path tests/test-install.ps1 -Tag "Runtime"

BeforeAll {
    $ScriptPath = Join-Path (Join-Path $PSScriptRoot "..") "install.ps1"
    $ScriptContent = Get-Content $ScriptPath -Raw
    $RepoRoot = Join-Path $PSScriptRoot ".."
    $InputsSource = Join-Path $RepoRoot "inputs"

    # Remove the final "Main" call so we can dot-source without executing
    $SafeContent = $ScriptContent -replace '(?m)^Main\s*$', ''
    $TempScript = Join-Path $TestDrive "install-testable.ps1"
    Set-Content -Path $TempScript -Value $SafeContent
    . $TempScript
}

Describe "Script syntax" -Tag "Static" {
    It "parses without errors" {
        $ScriptPath = Join-Path (Join-Path $PSScriptRoot "..") "install.ps1"
        $errors = $null
        [System.Management.Automation.Language.Parser]::ParseFile($ScriptPath, [ref]$null, [ref]$errors)
        $errors.Count | Should -Be 0
    }

    It "does not use Join-Path with more than 2 positional arguments" {
        $ScriptPath = Join-Path (Join-Path $PSScriptRoot "..") "install.ps1"
        $content = Get-Content $ScriptPath -Raw
        # Match Join-Path with 3+ quoted string arguments (PS 5.1 incompatible)
        $content | Should -Not -Match 'Join-Path\s+\S+\s+"[^"]+"\s+"[^"]+"'
    }
}

Describe "Install-Binary parameters" -Tag "Static" {
    It "accepts Version, Arch, and InstallDir parameters" {
        $cmd = Get-Command Install-Binary
        $cmd.Parameters.Keys | Should -Contain "Version"
        $cmd.Parameters.Keys | Should -Contain "Arch"
        $cmd.Parameters.Keys | Should -Contain "InstallDir"
    }
}

Describe "Input preset functions" -Tag "Static" {
    It "defines config dir and input preset installation functions" {
        Get-Command Get-ConfigDir | Should -Not -BeNullOrEmpty
        Get-Command Install-InputPresets | Should -Not -BeNullOrEmpty
    }
}

Describe "Get-Arch" -Tag "Runtime" {
    It "returns x86_64 on AMD64" {
        if ($env:PROCESSOR_ARCHITECTURE -eq "AMD64") {
            Get-Arch | Should -Be "x86_64"
        } else {
            { Get-Arch } | Should -Throw
        }
    }
}

Describe "Get-InstallDir" -Tag "Runtime" {
    Context "with BACKSCROLL_INSTALL_DIR set" {
        It "returns the custom directory" {
            $customDir = Join-Path $TestDrive "custom-install"
            $env:BACKSCROLL_INSTALL_DIR = $customDir
            try {
                $result = Get-InstallDir
                $result | Should -Be $customDir
            } finally {
                Remove-Item Env:\BACKSCROLL_INSTALL_DIR -ErrorAction SilentlyContinue
            }
        }

        It "returns exactly one value (not an array)" {
            $customDir = Join-Path $TestDrive "single-value"
            $env:BACKSCROLL_INSTALL_DIR = $customDir
            try {
                $result = Get-InstallDir
                $result | Should -BeOfType [string]
                @($result).Count | Should -Be 1
            } finally {
                Remove-Item Env:\BACKSCROLL_INSTALL_DIR -ErrorAction SilentlyContinue
            }
        }
    }

    Context "with default path" {
        It "returns a path ending in backscroll\bin" {
            $originalInstallDir = $env:BACKSCROLL_INSTALL_DIR
            $originalLocalAppData = $env:LOCALAPPDATA
            $env:BACKSCROLL_INSTALL_DIR = ""
            $env:LOCALAPPDATA = Join-Path $TestDrive "local-app-data"
            try {
                $result = Get-InstallDir
                $result | Should -Match "backscroll[/\\]bin$"
            } finally {
                $env:BACKSCROLL_INSTALL_DIR = $originalInstallDir
                $env:LOCALAPPDATA = $originalLocalAppData
            }
        }

        It "returns exactly one string (PS 5.1 Join-Path compat)" {
            $originalInstallDir = $env:BACKSCROLL_INSTALL_DIR
            $originalLocalAppData = $env:LOCALAPPDATA
            $env:BACKSCROLL_INSTALL_DIR = ""
            $env:LOCALAPPDATA = Join-Path $TestDrive "local-app-data-single"
            try {
                $result = Get-InstallDir
                @($result).Count | Should -Be 1
                $result | Should -BeOfType [string]
            } finally {
                $env:BACKSCROLL_INSTALL_DIR = $originalInstallDir
                $env:LOCALAPPDATA = $originalLocalAppData
            }
        }
    }
}

Describe "Get-ConfigDir" -Tag "Runtime" {
    It "uses BACKSCROLL_CONFIG_DIR when set" {
        $customDir = Join-Path $TestDrive "custom-config"
        $env:BACKSCROLL_CONFIG_DIR = $customDir
        try {
            Get-ConfigDir | Should -Be $customDir
        } finally {
            Remove-Item Env:\BACKSCROLL_CONFIG_DIR -ErrorAction SilentlyContinue
        }
    }

    It "uses APPDATA by default" {
        $originalConfigDir = $env:BACKSCROLL_CONFIG_DIR
        $originalAppData = $env:APPDATA
        $appData = Join-Path $TestDrive "appdata"
        $env:BACKSCROLL_CONFIG_DIR = ""
        $env:APPDATA = $appData
        try {
            Get-ConfigDir | Should -Be $appData
        } finally {
            $env:BACKSCROLL_CONFIG_DIR = $originalConfigDir
            $env:APPDATA = $originalAppData
        }
    }
}

Describe "Install-InputPresets" -Tag "Runtime" {
    It "copies presets to BACKSCROLL_CONFIG_DIR\backscroll\inputs" {
        $configDir = Join-Path $TestDrive "config-copy"
        $env:BACKSCROLL_CONFIG_DIR = $configDir
        try {
            Install-InputPresets -Version "v0.2.3" -SourceDir $InputsSource
            Test-Path (Join-Path (Join-Path $configDir "backscroll") "inputs") | Should -BeTrue
            Test-Path (Join-Path (Join-Path (Join-Path $configDir "backscroll") "inputs") "claude.inputs.toml") | Should -BeTrue
            Test-Path (Join-Path (Join-Path (Join-Path $configDir "backscroll") "inputs") "pi.inputs.toml") | Should -BeTrue
        } finally {
            Remove-Item Env:\BACKSCROLL_CONFIG_DIR -ErrorAction SilentlyContinue
        }
    }

    It "does not overwrite existing presets by default" {
        $configDir = Join-Path $TestDrive "config-skip"
        $inputsDir = Join-Path (Join-Path $configDir "backscroll") "inputs"
        New-Item -ItemType Directory -Path $inputsDir -Force | Out-Null
        $claudePreset = Join-Path $inputsDir "claude.inputs.toml"
        Set-Content -Path $claudePreset -Value "user edit"
        $env:BACKSCROLL_CONFIG_DIR = $configDir
        try {
            Install-InputPresets -Version "v0.2.3" -SourceDir $InputsSource
            Get-Content $claudePreset -Raw | Should -Match "user edit"
        } finally {
            Remove-Item Env:\BACKSCROLL_CONFIG_DIR -ErrorAction SilentlyContinue
        }
    }

    It "overwrites existing presets with BACKSCROLL_FORCE_INPUTS=1" {
        $configDir = Join-Path $TestDrive "config-force"
        $inputsDir = Join-Path (Join-Path $configDir "backscroll") "inputs"
        New-Item -ItemType Directory -Path $inputsDir -Force | Out-Null
        $claudePreset = Join-Path $inputsDir "claude.inputs.toml"
        Set-Content -Path $claudePreset -Value "user edit"
        $env:BACKSCROLL_CONFIG_DIR = $configDir
        $env:BACKSCROLL_FORCE_INPUTS = "1"
        try {
            Install-InputPresets -Version "v0.2.3" -SourceDir $InputsSource
            Get-Content $claudePreset -Raw | Should -Match 'id = "claude"'
        } finally {
            Remove-Item Env:\BACKSCROLL_CONFIG_DIR -ErrorAction SilentlyContinue
            Remove-Item Env:\BACKSCROLL_FORCE_INPUTS -ErrorAction SilentlyContinue
        }
    }
}
