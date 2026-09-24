Output docs/assets/images/demo.gif
Output docs/assets/images/demo.mp4

Set Shell "fish"
Set FontSize 14
Set FontFamily "BlexMono Nerd Font"
Set Width 1400
Set Height 800
Set Padding 12
Set Theme { "name": "RubyLLM Dark", "black": "#171315", "red": "#b30000", "green": "#759d82", "yellow": "#d8a07a", "blue": "#7aa2c8", "magenta": "#d4b4a5", "cyan": "#6aa6a6", "white": "#f5ede8", "brightBlack": "#4a3b42", "brightRed": "#c9271e", "brightGreen": "#a0d2a8", "brightYellow": "#ffb074", "brightBlue": "#9fc2e6", "brightMagenta": "#e0c8bc", "brightCyan": "#8bc4c4", "brightWhite": "#f5ede8", "background": "#171315", "foreground": "#f5ede8", "selection": "#3a2f35", "cursor": "#f5ede8" }

Hide
Type "set -e NO_COLOR; set -e FORCE_COLOR; set -e CLICOLOR_FORCE; set -gx TERM xterm-256color; set -gx COLORTERM truecolor; set -gx CLICOLOR 1; clear"
Enter
Sleep 300ms
Show

Type "hyprmoncfg"
Enter
Sleep 2s

# Arrange monitors directly on the canvas
Right 3
Sleep 1s
Left 3
Sleep 1s

# Select the other monitor
Type "]"
Sleep 1s

# Focus Display settings and browse its fields
Tab
Sleep 1s
Down 2
Sleep 1s

# Switch the inspector to Color
Type "]"
Sleep 1s
Down 2
Sleep 1s

# Return to Display, then the canvas
Type "["
Sleep 1s
Tab
Sleep 1s

# Restore the live layout before leaving the editor
Type "r"
Sleep 1.5s

# Browse saved hardware-aware profiles
Type "3"
Sleep 1s
Down 2
Sleep 1s
Up
Sleep 1s

# Preview workspace planning
Type "2"
Sleep 1s
Down
Sleep 1s
Right
Sleep 1s
Left
Sleep 1s

# Reset the draft and finish on the spatial editor
Type "r"
Sleep 1s
Type "1"
Sleep 2s

# Done
Sleep 1s
