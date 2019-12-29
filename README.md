# i3gator

Yes, it's yet another i3 layout manager. But this one doesn't even pretend to
do any declarative management of your i3 layouts, it's actually just a helper
for writing soft of scripts what should be done to create a layout.

Layout example:
```yaml
workspaces:
    w:
        - cwd: ~/go/src/
        - terminal-new
        - terminal-write: make run
        - terminal-enter

        - i3: split vertical

        - terminal-new
        - terminal-write: make debug

        - terminal-new
        - terminal-write: make nginx
        - terminal-enter

        - terminal-wait

        - i3: move left

# vim: ft=yaml
```

i3gator doesn't know much about these terminal-* identifiers, it only knows
how to work with workspace, `cwd` and `i3` things.

So, it requires a list of definition of these programs,
your ~/.config/i3gator/i3gator.conf might look like:

```yaml
programs:
    terminal-new:
        exec:
          - "terminal"

    terminal-write:
        exec:
          - "tmux"
          - "send-keys"
          - "-t"
          - "<terminal-new>"
          - "-l"
          - "<args>"

    terminal-enter:
        exec:
          - "tmux"
          - "send-keys"
          - "-t"
          - "<terminal-new>"
          - "Enter"

    terminal-wait:
        exec:
            - "xdotool"
            - "search"
            - "--sync"
            - "--name"
            - "<terminal-new>"
```

`terminal` binary in this example is expected to return a name of tmux session
which will be used afterwards for sending keys to tmux session. Every stdout is
written into a hashmap and then can be used by its identifier like
`<terminal-new>`.

There are only few things support out of the box so far:
- `cwd` changes current working directory;
- `i3` calls i3-msg and passes args.

Everything else is up to you.

i3gator doesn't give a heck about what you already have on the workspace, it
will just run the commands. *Worse is better*.

# Installation

```
go get github.com/kovetskiy/i3gator
```

# Usage

Put your main config in `~/.config/i3gator/i3gator.conf` and then push your
layouts to directory `~/.config/i3gator/layouts/` with suffix `.conf`.

Call the gator like `i3gator job` and it will apply layout from
`~/.config/i3gator/layouts/job.conf`.


# Questions

Didn't get something from the doc? It's okay, I wrote it in a few minutes
because I don't expect anyone except myself and close friends to use this
software, but if you still decided to give it a try and something is not
working, I'll be glad to assist you and will finally update the doc.
