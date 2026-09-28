# festival doctor: the command's status board, then the same board in the hub.
#
# Record with: just vhs record docs/demos/festival-doctor.tape
# Requires: just build (bin/festival).

Output docs/demos/festival-doctor.gif

Set Shell "bash"
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 15
Set Width 1100
Set Height 780
Set Padding 16
Set Margin 0
Set BorderRadius 0
Set Framerate 24
Set TypingSpeed 40ms
Set Theme {"name":"termcast","background":"#0d0d11","foreground":"#f8f8f2","cursor":"#F2721C","selection":"#2a2a33","black":"#16161a","red":"#ff5f57","green":"#50fa7b","yellow":"#febc2e","blue":"#57a6ff","magenta":"#bd93f9","cyan":"#8be9fd","white":"#f8f8f2","brightBlack":"#6b7280","brightRed":"#ff6e67","brightGreen":"#69ff94","brightYellow":"#ffd23f","brightBlue":"#7ab8ff","brightMagenta":"#d0a9ff","brightCyan":"#a4f0ff","brightWhite":"#ffffff"}

Hide
Type ". docs/demos/vhs-fake-prefix.sh shadow" Enter
# The prefix script enables reduced motion, which collapses the splash to one
# frame. Leave the splash up so Enter skips it instead of opening the first
# home row.
Type "unset FESTIVAL_REDUCED_MOTION" Enter
Type "clear" Enter
Show

Type "festival doctor" Sleep 200ms Enter
Sleep 4s

Type "festival" Sleep 300ms Enter
Sleep 1600ms
Type "8"
Sleep 4s

Type "q"
Sleep 500ms
