# Herdr

`config.toml` configures Herdr TUI in Ghostty. `app/` configures the Herdr desktop app.

## Desktop worktree shortcuts

| Shortcut | Action |
| --- | --- |
| Cmd+Shift+N | Create a worktree from the current repository. |
| Cmd+O | Open an existing worktree of the current repository. |

Open a repository in a Herdr space before using either shortcut. Each helper opens Herdr's built-in worktree dialog for the focused terminal's folder, including spaces on remote machines.

Run the config installer on macOS:

```sh
make install-configs
```

To install only the desktop worktree shortcuts, run:

```sh
./installers/install-herdr-app.sh
```

The installer symlinks `app/keys.json` and the two helper mods into `~/.config/herdr-app`. Set `HERDR_APP_HOME` to install into a different Herdr app directory. Herdr loads changes while it runs. Run the installer again after adding files or moving the repository.

Before replacing a regular `keys.json`, the installer saves it as `keys.json.before-dotfiles-<timestamp>` in the same directory. Other desktop files, including settings and layouts, stay under Herdr's control.

`app/keys.json` contains the macOS defaults from Herdr 0.1.16 plus the two worktree shortcuts. Herdr treats this file as a complete keymap. After an app upgrade, add any desired new default shortcuts to this file.
