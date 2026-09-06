# festival leftover-shadow TUI: package prefix plus leftover camp/fest.
#
# Record with: just vhs record docs/demos/festival-shadow.tape
# Requires: just build (bin/festival). Leftover stubs have no bundle line.

Output docs/demos/festival-shadow.gif

Set Shell "bash"
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 15
Set Width 1100
Set Height 720
Set Padding 16
Set Margin 0
Set BorderRadius 0
Set Framerate 24
Set TypingSpeed 40ms
Set Theme {"name":"termcast","background":"#0d0d11","foreground":"#f8f8f2","cursor":"#F2721C","selection":"#2a2a33","black":"#16161a","red":"#ff5f57","green":"#50fa7b","yellow":"#febc2e","blue":"#57a6ff","magenta":"#bd93f9","cyan":"#8be9fd","white":"#f8f8f2","brightBlack":"#6b7280","brightRed":"#ff6e67","brightGreen":"#69ff94","brightYellow":"#ffd23f","brightBlue":"#7ab8ff","brightMagenta":"#d0a9ff","brightCyan":"#a4f0ff","brightWhite":"#ffffff"}

Hide
Type ". docs/demos/vhs-fake-prefix.sh shadow" Enter
Type "clear" Enter
Show

Type "command -v festival; command -v camp" Enter
Sleep 500ms

Type "festival" Sleep 250ms Enter
Sleep 2000ms

# home: package card + leftover shadow lines (v0.5.0 / dev)
Sleep 2400ms

# doctor (menu row 8): path_shadowing warn lists leftover paths
Type "8"
Sleep 2400ms
Escape
Sleep 700ms

Type "q"
Sleep 400ms

Type "test ! -e $FESTIVAL_HOME/state.db && echo HOME_CLEAN" Enter
Sleep 700ms
