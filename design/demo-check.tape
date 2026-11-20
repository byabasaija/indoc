# The "How it works" demo: indoc failing its own docs when a cited function changes.
# Record with design/record-demo.sh, which runs this in a throwaway copy of
# the repository; never run it in your working tree.

Output demo-check.gif

Set Shell "bash"
Set FontSize 22
Set Width 1400
Set Height 540
Set Padding 32
Set Theme "GitHub Dark"
Set TypingSpeed 40ms
Set WindowBar Colorful

Hide
Type "export PS1='\[\e[1;34m\]$\[\e[0m\] ' GIT_PAGER=cat && clear" Enter
Show

Type "# indoc's own docs cite the code they describe" Enter
Sleep 300ms
Type "grep -o 'It never writes.*' documentation/guides/ci.md" Enter
Sleep 1.5s
Type "indoc check" Enter
Sleep 2.5s

# The edit happens off camera; the diff shows what changed.
Hide
Type "sed -i.bak 's/strict, warn, asJSON bool) error {/mode string, asJSON bool) error {/' internal/cli/check.go && rm internal/cli/check.go.bak && clear" Enter
Sleep 500ms
Show

Type "# someone changes runCheck's parameters" Enter
Sleep 300ms
Type "git diff -U0 --color=always | grep 'func runCheck'" Enter
Sleep 2.5s
Type "indoc check" Enter
Sleep 3s
Type "echo $?   # non-zero: CI fails until the page is updated" Enter
Sleep 5s
