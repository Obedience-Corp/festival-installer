# festival first run: setup card, the getting started tour, PATH with consent
#
# Record with: just vhs record docs/demos/festival-first-run.tape
# Requires: just build
#
# The env helper gives this tape its own HOME and FESTIVAL_HOME. That matters
# here more than on the other tapes, because the PATH step writes to a shell rc
# file and this tape shows it being written.
#
# No network install. Step 1 of the tour stays untouched: a real suite install
# varies in duration and output run to run, which is what makes a recording
# flaky. Step 2 is local, deterministic, and the one that needs consent.

Output docs/demos/festival-first-run.gif

Set Shell "bash"
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 15
Set Width 1100
Set Height 800
Set Padding 16
Set Margin 0
Set BorderRadius 0
Set Framerate 24
Set TypingSpeed 50ms
Set Theme {"name":"termcast","background":"#0d0d11","foreground":"#f8f8f2","cursor":"#F2721C","selection":"#2a2a33","black":"#16161a","red":"#ff5f57","green":"#50fa7b","yellow":"#febc2e","blue":"#57a6ff","magenta":"#bd93f9","cyan":"#8be9fd","white":"#f8f8f2","brightBlack":"#6b7280","brightRed":"#ff6e67","brightGreen":"#69ff94","brightYellow":"#ffd23f","brightBlue":"#7ab8ff","brightMagenta":"#d0a9ff","brightCyan":"#a4f0ff","brightWhite":"#ffffff"}

Hide
Type ". docs/demos/vhs-hub-env.sh fresh" Enter
Type "clear" Enter
Show

# nothing installed, nothing configured
Type "festival" Sleep 300ms Enter
Sleep 1800ms

# home on a fresh machine: the setup card, not empty booths
Sleep 3000ms

# Getting started is home row 1
Type "1"
Sleep 3000ms

# four steps, all still to do
Sleep 1500ms

# step 2 puts the managed bin dir on PATH
Down Sleep 800ms
Enter
Sleep 2500ms

# the exact rc file and the exact block, before anything is written
Sleep 2000ms
Type "y"
Sleep 2500ms

# step 2 is written but not active until the shell restarts
Sleep 2000ms

Escape
Sleep 2000ms

Type "q"
Sleep 600ms
