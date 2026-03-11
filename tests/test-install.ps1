# Pester tests for install.ps1
# Run with: Invoke-Pester -Path tests/test-install.ps1

BeforeAll {
    $ScriptPath = Join-Path $PSScriptRoot ".." "install.ps1"
    $ScriptContent = Get-Content $ScriptPath -Raw

    # Remove the final "Main" call so we can dot-source without executing
    $SafeContent = $ScriptContent -replace '(?m)^Main\s*$', ''
    $TempScript = Join-Path $TestDrive "install-testable.ps1"
    Set-Content -Path $TempScript -Value $SafeContent
    . $TempScript
}

Describe "Get-Arch" {
    It "returns x86_64 on AMD64" {
        # Only runs meaningfully on AMD64, but validates the function exists
        if ($env:PROCESSOR_ARCHITECTURE -eq "AMD64") {
            Get-Arch | Should -Be "x86_64"
        } else {
            { Get-Arch } | Should -Throw
        }
    }
}

Describe "Get-InstallDir" {
    Context "with BACKSCROLL_INSTALL_DIR set" {
        It "returns the custom directory" {
            $customDir = Join-Path $TestDrive "custom-install"
            $env:BACKSCROLL_INSTALL_DIR = $customDir
            try {
                $result = Get-InstallDir
                $result | Should -Be $customDir
            } finally {
                Remove-Item Env:\BACKSCROLL_INSTALL_DIR
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
                Remove-Item Env:\BACKSCROLL_INSTALL_DIR
            }
        }
    }

    Context "with default path" {
        It "returns a path ending in backscroll\bin" {
            $originalEnv = $env:BACKSCROLL_INSTALL_DIR
            $env:BACKSCROLL_INSTALL_DIR = ""
            try {
                $result = Get-InstallDir
                $result | Should -Match "backscroll[/\\]bin$"
            } finally {
                $env:BACKSCROLL_INSTALL_DIR = $originalEnv
            }
        }

        It "returns exactly one string (PS 5.1 Join-Path compat)" {
            $originalEnv = $env:BACKSCROLL_INSTALL_DIR
            $env:BACKSCROLL_INSTALL_DIR = ""
            try {
                $result = Get-InstallDir
                @($result).Count | Should -Be 1
                $result | Should -BeOfType [string]
            } finally {
                $env:BACKSCROLL_INSTALL_DIR = $originalEnv
            }
        }
    }
}

Describe "Script syntax" {
    It "parses without errors" {
        $ScriptPath = Join-Path $PSScriptRoot ".." "install.ps1"
        $errors = $null
        [System.Management.Automation.Language.Parser]::ParseFile($ScriptPath, [ref]$null, [ref]$errors)
        $errors.Count | Should -Be 0
    }

    It "does not use Join-Path with more than 2 positional arguments" {
        $ScriptPath = Join-Path $PSScriptRoot ".." "install.ps1"
        $content = Get-Content $ScriptPath -Raw
        # Match Join-Path with 3+ quoted string arguments (PS 5.1 incompatible)
        $content | Should -Not -Match 'Join-Path\s+\S+\s+"[^"]+"\s+"[^"]+"'
    }
}

Describe "Install-Binary parameters" {
    It "accepts Version, Arch, and InstallDir parameters" {
        $cmd = Get-Command Install-Binary
        $cmd.Parameters.Keys | Should -Contain "Version"
        $cmd.Parameters.Keys | Should -Contain "Arch"
        $cmd.Parameters.Keys | Should -Contain "InstallDir"
    }
}
