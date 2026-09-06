# festival install channels: pick a channel, shell/PATH guidance, installed list
#
# Record with: just vhs record docs/demos/festival-channels.tape
#
# This tape was called festival-tour.tape before there was a getting started
# tour. It never showed one. The tour is docs/demos/festival-first-run.tape.
#
# Height is 800 for the same reason as festival-home.tape: the setup card
# pushes the home screen past the 33 rows a 720px frame gives.

Output docs/demos/festival-channels.gif

Set Shell "bash"
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 15
Set Width 1100
Set Height 800
Set Padding 16
Set Margin 0
Set BorderRadius 0
Set Framerate 24
Set TypingSpeed 45ms
Set Theme {"name":"termcast","background":"#0d0d11","foreground":"#f8f8f2","cursor":"#F2721C","selection":"#2a2a33","black":"#16161a","red":"#ff5f57","green":"#50fa7b","yellow":"#febc2e","blue":"#57a6ff","magenta":"#bd93f9","cyan":"#8be9fd","white":"#f8f8f2","brightBlack":"#6b7280","brightRed":"#ff6e67","brightGreen":"#69ff94","brightYellow":"#ffd23f","brightBlue":"#7ab8ff","brightMagenta":"#d0a9ff","brightCyan":"#a4f0ff","brightWhite":"#ffffff"}

Hide
Type ". docs/demos/vhs-hub-env.sh fresh" Enter
Type "clear" Enter
Show

Type "festival" Sleep 250ms Enter

# The boot splash swallows anything that is not enter, space or escape, so
# the first menu key has to wait for the home screen.
Sleep 2600ms

# Install Festival suite is menu row 2 now that Getting started is row 1.
# Open the channel picker, cycle it, and leave without a network install.
Type "2"
Sleep 1600ms
Right Sleep 500ms
Right Sleep 500ms
Left Sleep 500ms
Sleep 1200ms
Escape
Sleep 800ms

# shell / PATH guidance (menu row 9)
Type "9"
Sleep 2200ms
Escape
Sleep 700ms

# installed packages list (menu row 4)
Type "4"
Sleep 1800ms
Escape
Sleep 900ms

Type "q"
Sleep 500ms
