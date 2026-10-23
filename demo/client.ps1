# client.ps1 - drives iconplugin.exe over MCP stdio and prints what each step returns.
# ASCII only (Windows PowerShell reads this file as ANSI).
param(
    [string]$Exe = "$PSScriptRoot\..\iconplugin.exe",
    [string]$OutDir = "$PSScriptRoot\..\demo"
)
$ErrorActionPreference = 'Stop'
New-Item -ItemType Directory -Force $OutDir | Out-Null
$psi = New-Object System.Diagnostics.ProcessStartInfo
$psi.FileName = $Exe
$psi.UseShellExecute = $false
$psi.RedirectStandardInput = $true
$psi.RedirectStandardOutput = $true
$psi.RedirectStandardError = $true
$psi.CreateNoWindow = $true
$p = [System.Diagnostics.Process]::Start($psi)
$script:id = 0

function Send([string]$method, $params) {
    $script:id++
    $msg = @{ jsonrpc = '2.0'; id = $script:id; method = $method }
    if ($null -ne $params) { $msg.params = $params }
    $p.StandardInput.WriteLine(($msg | ConvertTo-Json -Compress -Depth 20))
    $p.StandardInput.Flush()
    $task = $p.StandardOutput.ReadLineAsync()
    if (-not $task.Wait(45000)) { throw "timeout waiting for $method" }
    $line = $task.Result
    if (-not $line) { throw "no response for $method" }
    return $line | ConvertFrom-Json
}

function Notify([string]$method) {
    $p.StandardInput.WriteLine((@{ jsonrpc = '2.0'; method = $method } | ConvertTo-Json -Compress))
    $p.StandardInput.Flush()
}

function Text($response) {
    return [string]$response.result.content[0].text
}

try {
    $init = Send 'initialize' @{ protocolVersion = '2025-06-18'; capabilities = @{}; clientInfo = @{ name = 'demo'; version = '1.0' } }
    Write-Output ("initialize   -> {0} {1}, protocol {2}" -f $init.result.serverInfo.name, $init.result.serverInfo.version, $init.result.protocolVersion)
    Notify 'notifications/initialized'

    $tools = Send 'tools/list' $null
    Write-Output ("tools/list   -> {0} tools: {1}" -f $tools.result.tools.Count, (($tools.result.tools | ForEach-Object { $_.name }) -join ', '))

    $search = Send 'tools/call' @{ name = 'search_icons'; arguments = @{ query = 'database'; limit = 5 } }
    $s = Text $search | ConvertFrom-Json
    Write-Output ("search_icons -> {0} results for 'database' (total {1})" -f $s.count, $s.total)
    $s.icons | ForEach-Object { Write-Output ("               {0}  [{1}, {2}]" -f $_.icon, $_.collection, $_.license) }

    $sets = Send 'tools/call' @{ name = 'list_collections'; arguments = @{ query = 'lucide'; limit = 5 } }
    $l = Text $sets | ConvertFrom-Json
    Write-Output ("list_collections -> {0} match(es)" -f $l.count)
    $l.collections | ForEach-Object { Write-Output ("               {0}  {1} ({2} icons, {3})" -f $_.prefix, $_.name, $_.total, $_.license.spdx) }

    $one = Send 'tools/call' @{ name = 'get_collection'; arguments = @{ prefix = 'lucide'; limit = 5 } }
    $c = Text $one | ConvertFrom-Json
    Write-Output ("get_collection -> {0}: {1} icons, {2} categories" -f $c.title, $c.total, $c.categories.PSObject.Properties.Count)
    $c.icons | ForEach-Object { Write-Output ("               {0}" -f $_) }

    $svg = Send 'tools/call' @{ name = 'get_icon'; arguments = @{ icon = 'lucide:database'; color = '#f5b64a' } }
    $markup = Text $svg
    $svgPath = Join-Path $OutDir 'lucide-database.svg'
    [IO.File]::WriteAllText($svgPath, $markup)
    Write-Output ("get_icon     -> lucide:database, {0} bytes, starts with {1}" -f $markup.Length, $markup.Substring(0, [Math]::Min(40, $markup.Length)))
    Write-Output ("               saved to {0}" -f $svgPath)

    $uri = Send 'tools/call' @{ name = 'get_icon'; arguments = @{ icon = 'mdi:home'; format = 'data-uri' } }
    $data = Text $uri
    Write-Output ("get_icon     -> mdi:home as data URI, {0} bytes, prefix {1}" -f $data.Length, $data.Substring(0, 26))

    $bad = Send 'tools/call' @{ name = 'get_icon'; arguments = @{ icon = 'lucide:does-not-exist' } }
    Write-Output ("error path   -> isError={0}: {1}" -f $bad.result.isError, (Text $bad))
} finally {
    $p.StandardInput.Close()
    if (-not $p.WaitForExit(5000)) { $p.Kill() }
    $err = $p.StandardError.ReadToEnd()
    if ($err.Trim()) { Write-Output ("server stderr: " + $err.Trim()) }
}
