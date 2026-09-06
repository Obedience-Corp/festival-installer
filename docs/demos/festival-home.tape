# festival home TUI: boot splash, fire ambient, multi-activity booths, menu
#
# Record with: just vhs record docs/demos/festival-home.tape
# Requires: just build  (the env helper copies bin/festival into a fake home)
#
# Height is 800, not 720. The first-run setup card makes the home screen 34
# rows, and a 720px frame at font size 15 only gives 33, which scrolls the
# header off the top of the recording.

Output docs/demos/festival-home.gif

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

# bare festival opens the manager TUI
Type "festival" Sleep 300ms Enter

# boot splash with fire + tagline
Sleep 1800ms

# home: setup card + flame ambient + booths + menu
Sleep 3200ms

# walk the menu so booths heat with the cursor, down to Doctor (row 8)
Down Sleep 350ms
Down Sleep 350ms
Down Sleep 350ms
Down Sleep 350ms
Down Sleep 350ms
Down Sleep 350ms
Down Sleep 400ms

# linger on doctor, then open it
Sleep 900ms
Enter
Sleep 2200ms

# back home, show the tagline once more
Escape
Sleep 1600ms

Type "q"
Sleep 600ms
