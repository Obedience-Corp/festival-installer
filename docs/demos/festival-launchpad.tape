# festival launchpad — open camp as a child tool, quit back to hub
#
# Record with: just vhs record docs/demos/festival-launchpad.tape
# Needs: just build, and camp on PATH (or in a PATH dir we append).

Output docs/demos/festival-launchpad.gif

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
# festival first, then host PATH so launchpad can resolve real camp/fest
Type "export PATH=$PWD/bin:/opt/homebrew/bin:$HOME/go/bin:$PATH" Enter
Type "export TERM=xterm-256color COLORTERM=truecolor COLORFGBG=15\;0" Enter
Type "export FESTIVAL_HOME=/tmp/festival-vhs-launch-$$" Enter
Type "mkdir -p $FESTIVAL_HOME" Enter
Type "export PS1=$'\033[38;5;208m❯\033[0m '" Enter
Type "command -v camp && command -v festival" Enter
Type "clear" Enter
Show

Type "festival" Sleep 300ms Enter
# boot + home
Sleep 2000ms

# open Launchpad (home item 9 → key 9)
Type "9"
Sleep 1800ms

# move to "Camp version" (catalog index 4)
Down Sleep 300ms
Down Sleep 300ms
Down Sleep 300ms
Down Sleep 400ms
Sleep 800ms

# launch child — camp version prints and exits; hub should return
Enter
Sleep 2500ms

# linger on hub with return banner
Sleep 2200ms

Type "q"
Sleep 500ms
