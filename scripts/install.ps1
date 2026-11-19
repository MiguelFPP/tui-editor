<#
.SYNOPSIS
    Installer for tui-editor on Windows.

.DESCRIPTION
    Downloads the release archive for the current architecture, verifies its
    SHA-256 checksum and installs tui-editor into a directory on the PATH.

.EXAMPLE
    irm https://raw.githubusercontent.com/MiguelFPP/tui-editor/main/scripts/install.ps1 | iex

.EXAMPLE
    .\install.ps1 -Version v0.3.0 -Dir C:\Tools\tui-editor
#>
[CmdletBinding()]
param(
    [string]$Version,
    [string]$Dir,
    [switch]$NoVerify,
    [switch]$NoPath
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$Repo = 'MiguelFPP/tui-editor'
$Releases = "https://github.com/$Repo/releases"
$Binary = 'tui-editor'

function Write-Step {
    param([string]$Message)
    Write-Host "→ $Message" -ForegroundColor Cyan
}

function Write-Warn {
    param([string]$Message)
    Write-Host "tui-editor: $Message" -ForegroundColor Yellow
}

function Resolve-Architecture {
    $arch = $env:PROCESSOR_ARCHITECTURE
    if ($env:PROCESSOR_ARCHITEW6432) {
        $arch = $env:PROCESSOR_ARCHITEW6432
    }

    switch ($arch) {
        'AMD64' { return 'amd64' }
        'ARM64' { return 'arm64' }
        default { throw "arquitectura no soportada: $arch" }
    }
}

function Resolve-Version {
    if ($Version) {
        return $Version.TrimStart('v')
    }

    $latest = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" `
        -Headers @{ 'User-Agent' = 'tui-editor-installer' }

    return $latest.tag_name.TrimStart('v')
}

function Resolve-InstallDir {
    if ($Dir) {
        return $Dir
    }

    if ($env:LOCALAPPDATA) {
        return (Join-Path $env:LOCALAPPDATA 'Programs\tui-editor')
    }

    if ($env:USERPROFILE) {
        return (Join-Path $env:USERPROFILE '.tui-editor\bin')
    }

    return (Join-Path ([System.IO.Path]::GetTempPath()) 'tui-editor')
}

function Add-ToUserPath {
    param([string]$Target)

    $current = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not $current) {
        $current = ''
    }

    $entries = $current -split ';' | Where-Object { $_ -ne '' }
    if ($entries -contains $Target) {
        return $false
    }

    $updated = (@($entries) + $Target) -join ';'
    [Environment]::SetEnvironmentVariable('Path', $updated, 'User')

    return $true
}

try {
    Write-Step 'detectando plataforma'
    $arch = Resolve-Architecture

    Write-Step 'resolviendo versión'
    $resolved = Resolve-Version
    $name = "${Binary}_${resolved}_windows_${arch}"
    $url = "$Releases/download/v$resolved/$name.zip"

    $target = Resolve-InstallDir

    Write-Host "  versión:    $resolved"
    Write-Host "  plataforma: windows/$arch"
    Write-Host "  destino:    $target"

    $work = Join-Path ([System.IO.Path]::GetTempPath()) ("tui-editor-" + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $work -Force | Out-Null

    try {
        Write-Step "descargando $name.zip"
        $archive = Join-Path $work "$name.zip"
        try {
            Invoke-WebRequest -Uri $url -OutFile $archive -UseBasicParsing
        }
        catch {
            throw "falló la descarga: $url (¿está publicada v$resolved? ver $Releases)"
        }

        if (-not $NoVerify) {
            $checksums = Join-Path $work 'checksums.txt'
            $downloaded = $true
            try {
                Invoke-WebRequest -Uri "$Releases/download/v$resolved/checksums.txt" `
                    -OutFile $checksums -UseBasicParsing
            }
            catch {
                $downloaded = $false
                Write-Warn 'checksums.txt no está disponible; se omite la verificación'
            }

            if ($downloaded) {
                $expected = $null
                foreach ($line in Get-Content $checksums) {
                    $parts = $line -split '\s+', 2
                    if ($parts.Count -eq 2 -and $parts[1].Trim() -eq "$name.zip") {
                        $expected = $parts[0].Trim()
                        break
                    }
                }

                if (-not $expected) {
                    Write-Warn "$name.zip no aparece en checksums.txt; se omite la verificación"
                }
                else {
                    Write-Step 'verificando checksum'
                    $actual = (Get-FileHash -Path $archive -Algorithm SHA256).Hash.ToLowerInvariant()
                    if ($actual -ne $expected.ToLowerInvariant()) {
                        throw "el checksum no coincide para $name.zip (esperado $expected, obtenido $actual)"
                    }
                }
            }
        }

        Write-Step 'extrayendo'
        $extracted = Join-Path $work 'extract'
        Expand-Archive -Path $archive -DestinationPath $extracted -Force

        $exe = Get-ChildItem -Path $extracted -Filter "$Binary.exe" -Recurse |
            Select-Object -First 1
        if (-not $exe) {
            throw "no se encontró $Binary.exe en el archivo"
        }

        New-Item -ItemType Directory -Path $target -Force | Out-Null
        $destination = Join-Path $target "$Binary.exe"

        # Copy next to the target and move it into place: Windows refuses to
        # overwrite a running executable, and a failed move leaves the previous
        # working binary untouched.
        $staged = Join-Path $target ".$Binary.new.$PID.exe"
        try {
            Copy-Item -Path $exe.FullName -Destination $staged -Force
            Move-Item -Path $staged -Destination $destination -Force
        }
        finally {
            Remove-Item -Path $staged -Force -ErrorAction SilentlyContinue
        }

        Write-Host "✓ instalado $Binary $resolved en $destination" -ForegroundColor Green

        if (-not $NoPath) {
            if (Add-ToUserPath -Target $target) {
                Write-Warn "se añadió $target al PATH de usuario; abre una terminal nueva para usar $Binary"
            }
        }

        try {
            & $destination --version
        }
        catch {
            Write-Warn "no se pudo ejecutar $destination --version"
        }
    }
    finally {
        Remove-Item -Path $work -Recurse -Force -ErrorAction SilentlyContinue
    }
}
catch {
    Write-Host "tui-editor: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
