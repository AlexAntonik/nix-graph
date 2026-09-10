Output demo.gif

Set Shell bash
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 20
Set Width 1400
Set Height 800
Set Padding 20
Set Framerate 15
Set TypingSpeed 40ms
Set Theme "Catppuccin Mocha"

# Launch on the current system closure
Sleep 1s
Type "nix-graph"
Sleep 300ms
Enter
Sleep 2s

# Help overlay
Type "?"
Sleep 3s
Escape
Sleep 1s

# Sort modes: closure / added / deps
Type "c"
Sleep 2s
Type "c"
Sleep 2s
Type "a"
Sleep 2s
Type "a"
Sleep 2s
Type "d"
Sleep 2s
Type "c"
Sleep 1s

# Browse: expand a package, drill in, jump to the bottom and back
Type "jj"
Space
Sleep 1s
Type "lll"
Sleep 2s

# Copy the hash of the selected package
Type "y"
Sleep 3s
Type "h"
Sleep 2s


# Invert the graph/filter
Type "f"
Type "firefox"
Enter
Sleep 400ms
Type "jj"
Sleep 300ms
Type "f"
Escape
Sleep 1s
Type "p"
Sleep 3s
Type "llllllll"
Sleep 3s
Type "p"
Sleep 3s
Type "llllllll"
Sleep 3s
Type "p"
Sleep 3s
Type "llllllll"
Sleep 3s
Type "p"
Sleep 3s
Escape
Sleep 1s

# Quit
Type "q"
Sleep 1s