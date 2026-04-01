Output docs/assets/images/demo.gif
Output docs/assets/images/demo.mp4

Set Shell "fish"
Set FontSize 14
Set FontFamily "BlexMono Nerd Font"
Set Width 1400
Set Height 800
Set Padding 12
Set Theme "tokyonight"

Type "hyprmoncfg"
Enter
Sleep 1.5s

# Move the selected monitor around
Up 3
Sleep 0.3s
Down 3
Sleep 0.5s
Left 2
Sleep 0.5s
Right 2
Sleep 0.5s

# Switch to the other monitor
Type "]"
Sleep 0.5s

# Move it too
Up 6
Sleep 0.3s
Down 6
Sleep 1s

# Enter the inspector
Enter
Sleep 0.5s

# Navigate to Mode and edit
Down
Sleep 0.5s
Enter
Sleep 1s
Down
Sleep 1s
Enter
Sleep 1s

# Navigate to Scale and edit
Down
Sleep 0.5s
Enter
Sleep 1s
Backspace 2
Type "5"
Sleep 0.5s
Enter
Sleep 1s

# Back to canvas
Tab
Sleep 0.5s

# Move display down
Up 2
Sleep 1s

# Save the profile
Type "s"
Sleep 0.8s

# Type a name
Backspace 11
Type "My Setup"
Sleep 0.5s
Enter
Sleep 1s
Type "y"
Sleep 2s
Type "y"
Sleep 1.5s

# Switch to profiles
Type "2"
Sleep 0.5s
Down 6
Sleep 1.5s

# Switch to workspaces
Type "3"
Sleep 0.5s
Down
Sleep 0.5s
Right
Sleep 0.5s
Left
Sleep 0.5s
Right
Sleep 0.5s
Left
Sleep 1s

# Apply
Type "a"
Sleep 3s
Type "n"

# Done
Sleep 1s
