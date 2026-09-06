# festival package-origin TUI: just-built hub in a fake helper-adjacent prefix.
#
# Record with: just vhs record docs/demos/festival-package.tape
# Requires: just build (bin/festival). Do not record against live /usr/bin/festival.
#
# Host /usr/bin on this machine owns camp/fest/festival (AUR). The setup script
# puts mkdir/clear on PATH via a coreutils-only dir so FlavorUnknown (no yay).

Output docs/demos/festival-package.gif

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
Type ". docs/demos/vhs-fake-prefix.sh package" Enter
Type "clear" Enter
Show

Type "command -v festival" Enter
Sleep 400ms

Type "festival" Sleep 250ms Enter
Sleep 1800ms

# home channel card (package, FlavorUnknown, no yay)
Sleep 2200ms

# doctor (menu row 8, one lower since Getting started took row 1)
Type "8"
Sleep 2200ms
Escape
Sleep 600ms

# shell / PATH (menu row 9)
Type "9"
Sleep 2000ms
Escape
Sleep 600ms

# installed packages (menu row 4)
Type "4"
Sleep 1600ms
Escape
Sleep 500ms

# How you installed (menu row 2). Do not confirm a network install.
Type "2"
Sleep 1800ms
Escape
Sleep 700ms

Type "q"
Sleep 400ms

Type "command -v festival" Enter
Sleep 300ms
Type "test ! -e $FESTIVAL_HOME/state.db && test ! -e $FESTIVAL_HOME/locks && echo HOME_CLEAN" Enter
Sleep 800ms
