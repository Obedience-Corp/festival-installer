#Requires -Version 7.0
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$Image = 'mcr.microsoft.com/windows/servercore:ltsc2025'
$TempRoot = if ($env:RUNNER_TEMP) { $env:RUNNER_TEMP } else { [System.IO.Path]::GetTempPath() }
$OutDir = Join-Path $TempRoot 'festival-installer-windows-proof'
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

$Suites = @(
    @{ Package = './internal/artifacts'; Exe = 'artifacts.test.exe'; Pattern = '^Test(ExtractZip|VerifySHA256_Mismatch)' },
    @{ Package = './internal/state/lock'; Exe = 'lock.test.exe'; Pattern = '^Test(AcquireRelease_HappyPath|SecondAcquireBlocksUntilRelease|Acquire_ContextCancel|CrashRecovery_KernelReleasesLock)$' },
    @{ Package = './internal/installer'; Exe = 'installer.test.exe'; Pattern = '^Test(Commit_PlacesFilesAndWritesReceipt|Commit_UpgradeReplacesPriorBinary|Commit_FailureRestoresPriorBinaries|BeginPrunesCompletedWindowsImageBackup|WindowsSuiteUpgradeRollbackOnLockedExecutable|WindowsRunningExecutableReplacedAndOldProcessCompletes)$' },
    @{ Package = './internal/app'; Exe = 'app.test.exe'; Pattern = '^Test(SuiteBinaryNamesForWindows|ProductExecutableNames|ShellInitSnippet_PowerShellQuotesBinDir|WindowsDoctorVerifiesThreeManagedExecutablesAcrossVersions|WindowsScoopShimOriginAndGuidance|WindowsScoopShimRejectsUnownedPaths)$' },
    @{ Package = './internal/metadata'; Exe = 'metadata.test.exe'; Pattern = '^TestParseManifestWindowsSuiteArtifact$' }
)

Push-Location $RepoRoot
try {
    foreach ($suite in $Suites) {
        $output = Join-Path $OutDir $suite.Exe
        & go test -c -tags container_fs -o $output $suite.Package
        if ($LASTEXITCODE -ne 0) { throw "compile $($suite.Package) failed ($LASTEXITCODE)" }
    }
} finally {
    Pop-Location
}

# Hosted Windows runners can expose docker.exe before the Windows Docker
# service has started. Do not misreport that startup race as a product failure.
$DockerService = Get-Service -Name docker -ErrorAction SilentlyContinue
if ($DockerService -and $DockerService.Status -ne 'Running') {
    Start-Service docker
}
$DockerReady = $false
for ($attempt = 0; $attempt -lt 10; $attempt++) {
    $serverVersion = & docker info --format '{{.ServerVersion}}' 2>$null
    if ($LASTEXITCODE -eq 0 -and $serverVersion) {
        $DockerReady = $true
        break
    }
    Start-Sleep -Seconds 5
}
if (-not $DockerReady) { throw 'Windows Docker daemon did not become ready' }

& docker pull $Image
if ($LASTEXITCODE -ne 0) { throw "pull $Image failed ($LASTEXITCODE)" }
$container = (& docker create $Image powershell.exe -NoProfile -Command 'Start-Sleep -Seconds 1800').Trim()
if ($LASTEXITCODE -ne 0 -or -not $container) { throw 'create Windows proof container failed' }
try {
    & docker start $container | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'start Windows proof container failed' }
    & docker exec $container cmd /S /C 'mkdir C:\proof'
    if ($LASTEXITCODE -ne 0) { throw 'create proof directory failed' }
    & docker cp "${OutDir}\." "${container}:C:\proof"
    if ($LASTEXITCODE -ne 0) { throw 'copy proof executables failed' }
    & docker exec --user ContainerUser $container whoami
    if ($LASTEXITCODE -ne 0) { throw 'ContainerUser identity proof failed' }

    foreach ($suite in $Suites) {
        $exe = 'C:\proof\' + $suite.Exe
        $output = & docker exec --user ContainerUser -e 'TEMP=C:\Users\ContainerUser\AppData\Local\Temp' -e 'TMP=C:\Users\ContainerUser\AppData\Local\Temp' $container $exe '-test.v' '-test.count=1' "-test.run=$($suite.Pattern)" 2>&1
        $exitCode = $LASTEXITCODE
        $output | ForEach-Object { Write-Host $_ }
        if ($exitCode -ne 0) { throw "$($suite.Package) native proof failed ($exitCode)" }
        if (@($output | Select-String -Pattern '^--- PASS:' ).Count -eq 0) {
            throw "$($suite.Package) native proof ran no tests"
        }
        if (@($output | Select-String -Pattern '^--- SKIP:' ).Count -ne 0) {
            throw "$($suite.Package) native proof skipped a selected test"
        }
    }
    Write-Host 'Windows container installer proof passed'
} finally {
    & docker rm -f $container | Out-Null
}
