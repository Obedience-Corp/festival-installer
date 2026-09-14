# Run festival, select Join our Discord, and open the real invite.
# Record: just vhs record docs/demos/festival-discord.tape
# Requires a desktop browser and open (macOS) or xdg-open (Linux).
# Uses the existing isolated demo home; /usr/bin and /bin supply the real opener.
# VHS captures the terminal, not the browser window.

Output docs/demos/festival-discord.gif

Set Shell "bash"
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 15
Set Width 1100
Set Height 860
Set Padding 16
Set Margin 0
Set BorderRadius 0
Set Framerate 24
Set TypingSpeed 50ms
Set Theme {"name":"termcast","background":"#0d0d11","foreground":"#f8f8f2","cursor":"#F2721C","selection":"#2a2a33","black":"#16161a","red":"#ff5f57","green":"#50fa7b","yellow":"#febc2e","blue":"#57a6ff","magenta":"#bd93f9","cyan":"#8be9fd","white":"#f8f8f2","brightBlack":"#6b7280","brightRed":"#ff6e67","brightGreen":"#69ff94","brightYellow":"#ffd23f","brightBlue":"#7ab8ff","brightMagenta":"#d0a9ff","brightCyan":"#a4f0ff","brightWhite":"#ffffff"}

Hide
Type ". docs/demos/vhs-hub-env.sh fresh" Enter
Type 'export PATH="${PATH}:/usr/bin:/bin"; clear' Enter
Show

Type "festival" Sleep 300ms Enter
Sleep 5s

# Getting started is selected. Discord sits immediately before Quit.
Down Sleep 180ms
Down Sleep 180ms
Down Sleep 180ms
Down Sleep 180ms
Down Sleep 180ms
Down Sleep 180ms
Down Sleep 180ms
Down Sleep 180ms
Down Sleep 180ms
Down Sleep 180ms
Sleep 2s
Enter
Sleep 5s
Escape
Sleep 1s
Type "q"
Sleep 500ms
