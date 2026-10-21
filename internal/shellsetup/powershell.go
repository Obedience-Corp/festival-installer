package shellsetup

// PowerShell loads profiles for -File/-Command as well as interactive prompts.
// Accept only prompt startup options. Unknown modes (including encoded or
// abbreviated commands, scripts, remoting and server modes) leave the session
// untouched. NoExit allows the bounded readiness probe to feed a real prompt
// through stdin, including a Windows session without an interactive desktop.
// Ordinary piped input without NoExit stays unmodified.
// CLI option reference: https://learn.microsoft.com/powershell/module/microsoft.powershell.core/about/about_pwsh
const powerShellInteractive = `  if ($Host.Name -ne 'ConsoleHost') { return $false }
  $startup = @([Environment]::GetCommandLineArgs() | Select-Object -Skip 1)
  $noExit = $false
  for ($i = 0; $i -lt $startup.Count; $i++) {
    $option = $startup[$i].ToLowerInvariant()
    if (-not ($option.StartsWith('-') -or $option.StartsWith('/'))) { return $false }
    $option = $option.Substring(1)
    if ($option.Length -ge 3 -and 'noexit'.StartsWith($option)) { $noExit = $true; continue }
    if ($option.Length -ge 3 -and ('nologo'.StartsWith($option) -or 'noprofile'.StartsWith($option) -or $option -eq 'noprofileloadtime')) { continue }
    if (@('i', 'interactive', 'l', 'login', 'sta', 'mta') -contains $option) { continue }
    if (($option.Length -ge 2 -and 'executionpolicy'.StartsWith($option)) -or $option -eq 'ep' -or ($option.Length -ge 1 -and 'windowstyle'.StartsWith($option)) -or @('wd', 'workingdirectory') -contains $option) {
      $i++
      if ($i -ge $startup.Count) { return $false }
      continue
    }
    return $false
  }
  return ($noExit -or ([Environment]::UserInteractive -and -not [Console]::IsInputRedirected))
`
