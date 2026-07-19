# festival manager tour — install channel, shell/PATH, list, quit
#
# Record with: just vhs record docs/demos/festival-tour.tape

Output docs/demos/festival-tour.gif

Set Shell "bash"
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 15
Set Width 1100
Set Height 720
Set Padding 16
Set Margin 0
Set BorderRadius 0
Set Framerate 24
Set TypingSpeed 45ms
Set Theme {"name":"termcast","background":"#0d0d11","foreground":"#f8f8f2","cursor":"#F2721C","selection":"#2a2a33","black":"#16161a","red":"#ff5f57","green":"#50fa7b","yellow":"#febc2e","blue":"#57a6ff","magenta":"#bd93f9","cyan":"#8be9fd","white":"#f8f8f2","brightBlack":"#6b7280","brightRed":"#ff6e67","brightGreen":"#69ff94","brightYellow":"#ffd23f","brightBlue":"#7ab8ff","brightMagenta":"#d0a9ff","brightCyan":"#a4f0ff","brightWhite":"#ffffff"}

Hide
Type `export PATH="$PWD/bin"` Enter
Type `export TERM=xterm-256color COLORTERM=truecolor COLORFGBG='15;0'` Enter
Type `export FESTIVAL_HOME="$(mktemp -d)/festival-home"` Enter
Type `export PS1='\033[38;5;208m❯\033[0m '` Enter
Type `clear` Enter
Show

Type "festival" Sleep 250ms Enter
Sleep 1500ms

# open install channel picker (do not run a real network install)
Enter
Sleep 1600ms
Right Sleep 500ms
Right Sleep 500ms
Left Sleep 500ms
Sleep 1200ms
Escape
Sleep 800ms

# shell / PATH guidance (menu item 8 → index 7, key 8)
Type "8"
Sleep 2200ms
Escape
Sleep 700ms

# installed packages list (key 3)
Type "3"
Sleep 1800ms
Escape
Sleep 900ms

Type "q"
Sleep 500ms
