param(
    [string]$EnvFile = (Join-Path $PSScriptRoot '.env')
)

$ErrorActionPreference = 'Stop'

function Import-DotEnv([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) {
        throw "Файл настроек не найден: $Path. Скопируй deploy/.env.example в deploy/.env."
    }

    $values = @{}
    foreach ($line in Get-Content -LiteralPath $Path) {
        $trimmed = $line.Trim().TrimStart([char]0xFEFF)
        if ($trimmed.Length -eq 0 -or $trimmed.StartsWith('#')) {
            continue
        }
        if ($trimmed.StartsWith('export ')) {
            $trimmed = $trimmed.Substring(7).TrimStart()
        }

        $parts = $trimmed -split '=', 2
        if ($parts.Count -ne 2) {
            continue
        }

        $name = $parts[0].Trim()
        $value = $parts[1].Trim()
        if ($value.Length -ge 2) {
            $first = $value[0]
            $last = $value[$value.Length - 1]
            if (($first -eq '"' -and $last -eq '"') -or ($first -eq "'" -and $last -eq "'")) {
                $value = $value.Substring(1, $value.Length - 2)
            }
        }

        if ($name -match '^[A-Za-z_][A-Za-z0-9_]*$') {
            $values[$name] = $value
        }
    }

    return $values
}

function Require-Setting($Settings, [string]$Name) {
    if (-not $Settings.ContainsKey($Name) -or [string]::IsNullOrWhiteSpace([string]$Settings[$Name])) {
        throw "Не задана обязательная настройка $Name в deploy/.env."
    }
    return [string]$Settings[$Name]
}

function Invoke-Checked([string]$Command, [string[]]$Arguments) {
    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Команда завершилась с ошибкой ($LASTEXITCODE): $Command $($Arguments -join ' ')"
    }
}

function ConvertTo-BashLiteral([string]$Value) {
    return "'" + $Value.Replace("'", "'\''") + "'"
}

function Invoke-SshWithInput([string[]]$SshArguments, [string]$RemoteCommand, [string]$InputText) {
    $InputText | & $script:SshExecutable @SshArguments $RemoteCommand
    if ($LASTEXITCODE -ne 0) {
        throw "SSH-команда завершилась с ошибкой ($LASTEXITCODE): $RemoteCommand"
    }
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$settings = Import-DotEnv $EnvFile

$rcloneRemote = Require-Setting $settings 'DEPLOY_RCLONE_REMOTE'
$remotePath = (Require-Setting $settings 'DEPLOY_REMOTE_PATH').TrimEnd('/')
$sshTarget = Require-Setting $settings 'DEPLOY_SSH_TARGET'
$sshPort = if ($settings.ContainsKey('DEPLOY_SSH_PORT') -and $settings['DEPLOY_SSH_PORT']) { [string]$settings['DEPLOY_SSH_PORT'] } else { '22' }
$sshKey = if ($settings.ContainsKey('DEPLOY_SSH_KEY')) { [string]$settings['DEPLOY_SSH_KEY'] } else { '' }
$goProxy = if ($settings.ContainsKey('DEPLOY_GOPROXY') -and -not [string]::IsNullOrWhiteSpace([string]$settings['DEPLOY_GOPROXY'])) { [string]$settings['DEPLOY_GOPROXY'] } else { 'https://proxy.golang.org,direct' }
$fileHostingToken = if ($settings.ContainsKey('FILE_HOSTING_TOKEN')) { [string]$settings['FILE_HOSTING_TOKEN'] } else { '' }

if (-not (Get-Command rclone -ErrorAction SilentlyContinue)) { throw 'Не найден rclone в PATH.' }
$sshExecutable = (Get-Command ssh -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty Source)
if ([string]::IsNullOrWhiteSpace($sshExecutable)) {
    $sshCandidates = @(
        'C:\Windows\System32\OpenSSH\ssh.exe',
        'C:\Windows\Sysnative\OpenSSH\ssh.exe'
    )
    $sshExecutable = $sshCandidates | Where-Object { [System.IO.File]::Exists($_) } | Select-Object -First 1
}
if ([string]::IsNullOrWhiteSpace($sshExecutable)) { throw 'Не найден ssh.exe. Установи OpenSSH Client или добавь ssh.exe в PATH.' }
if (-not (Get-Command git -ErrorAction SilentlyContinue)) { throw 'Не найден git в PATH.' }

$stagingPath = Join-Path ([System.IO.Path]::GetTempPath()) ('mc-build-updater-deploy-' + [guid]::NewGuid().ToString('N'))
$remoteDestination = "${rcloneRemote}:$remotePath"
$remoteRoot = ConvertTo-BashLiteral $remotePath
$remoteEnvPath = ConvertTo-BashLiteral "$remotePath/deploy/.env"
$remoteGoProxy = ConvertTo-BashLiteral $goProxy

$script:SshExecutable = $sshExecutable
$sshArguments = @('-p', $sshPort)
if (-not [string]::IsNullOrWhiteSpace($sshKey)) { $sshArguments += @('-i', $sshKey) }
$sshArguments += $sshTarget

try {
    New-Item -ItemType Directory -Path $stagingPath -Force | Out-Null

    $gitFiles = & git -C $repoRoot ls-files --cached --others --exclude-standard
    if ($LASTEXITCODE -ne 0) { throw 'Не удалось получить список файлов из Git.' }

    foreach ($relativePath in $gitFiles) {
        if ([string]::IsNullOrWhiteSpace($relativePath)) { continue }
        $sourcePath = Join-Path $repoRoot $relativePath
        $targetPath = Join-Path $stagingPath $relativePath
        New-Item -ItemType Directory -Path (Split-Path -Parent $targetPath) -Force | Out-Null
        Copy-Item -LiteralPath $sourcePath -Destination $targetPath -Force
    }

    Write-Host "Синхронизация $stagingPath -> $remoteDestination"
    Write-Host 'Режим rclone: sync (фиксированный)'
    Invoke-Checked 'rclone' @('sync', $stagingPath, $remoteDestination, '--exclude', 'deploy/.env', '--exclude', 'apps/file-hosting/files/.file-hosting.sqlite*')

    $remoteEnv = "FILE_HOSTING_TOKEN=$fileHostingToken`n"
    $writeEnvCommand = "mkdir -p $remoteRoot/deploy && cat > $remoteEnvPath"
    Write-Host 'Передача production-настроек Compose на сервер'
    Invoke-SshWithInput $sshArguments $writeEnvCommand $remoteEnv

    $remoteScript = @'
set -eu
ROOT="$1"
COMPOSE_FILE="$ROOT/apps/file-hosting/compose.yaml"
ENV_FILE="$ROOT/deploy/.env"

docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" down --remove-orphans
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" build --pull --build-arg GOPROXY=$2
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d --force-recreate --remove-orphans
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" ps
'@
    $remoteScript = $remoteScript -replace "`r?`n", "`n"

    Write-Host 'Пересоздание production Compose'
    Invoke-SshWithInput $sshArguments "bash -s -- $remoteRoot $remoteGoProxy" $remoteScript
    Write-Host 'Деплой завершён.' -ForegroundColor Green
} finally {
    if (Test-Path -LiteralPath $stagingPath) {
        Remove-Item -LiteralPath $stagingPath -Recurse -Force -ErrorAction SilentlyContinue
    }
}
