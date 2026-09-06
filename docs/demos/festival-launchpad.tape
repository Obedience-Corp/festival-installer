# festival launchpad: open camp as a child tool, quit back to hub
#
# Record with: just vhs record docs/demos/festival-launchpad.tape
# Needs: just build, and a real camp and fest on PATH. The env helper copies
# both into the recording's own directory so the operator's paths stay off
# screen, and refuses to run if either is missing.
#
# Not part of "just vhs all" for that reason.
#
# Launchpad is the tenth home row and has no digit: digits 1 through 9 address
# the first nine rows and 0 is Quit. The arrow walk below is deliberate.

Output docs/demos/festival-launchpad.gif

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
Type ". docs/demos/vhs-hub-env.sh tools" Enter
Type "clear" Enter
Show

Type "command -v camp; command -v fest" Enter
Sleep 900ms

Type "festival" Sleep 300ms Enter
# boot splash, then home. Arrow keys are dropped while the splash is up.
Sleep 2600ms

# walk down to Launchpad, the tenth row
Down Sleep 220ms
Down Sleep 220ms
Down Sleep 220ms
Down Sleep 220ms
Down Sleep 220ms
Down Sleep 220ms
Down Sleep 220ms
Down Sleep 220ms
Down Sleep 400ms
Sleep 700ms
Enter
Sleep 1800ms

# move to "Camp version" (catalog index 4)
Down Sleep 300ms
Down Sleep 300ms
Down Sleep 300ms
Down Sleep 400ms
Sleep 800ms

# launch child: camp version prints and exits, the hub captures the output
Enter
Sleep 3000ms
Sleep 1500ms

# q closes the capture and lands back on the launchpad
Type "q"
Sleep 1500ms

# esc returns to the hub home, still running, nothing relaunched
Escape
Sleep 2600ms

Type "q"
Sleep 500ms
