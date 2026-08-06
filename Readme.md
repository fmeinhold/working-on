# Working-On

## What it is

First and foremost it's useful to me - it's a tool to easly track my time with toggl track from the command line,
using different sources as tasks. At the moment toggl is the only source, but the source interface is there so
others can be added. It is also an excuse to learn go.

It has only been tested on MacOS and Linux so far.

## Install

Coming Soon, looking into homebrew taps.


## Setup

Run ```wo init``` - this will ask you for your toggl track api key (https://track.toggl.com/profile) and some other,
mostly date and time related questions. It checks the token before going any further, and lets you pick a workspace
and a default project from what the account actually has. The config is written to ~/.config/working_on/config.yaml.

If you would rather write it yourself, copy `config.example.yaml` there instead - it documents every setting.

To avoid the token appearing in your terminal scrollback:

```
wo init --token "$(pbpaste)"
```

## Adding Mappings

A mapping ties a name, and optionally a git repository, to a toggl project and task:

```yaml
mappings:
  - name: "EXAMPLE"
    toggl_pid: 12345678
    toggl_task: 87654321   # optional
    git: git@github.com:you/example.git
```

The `git` entry is matched against `git config --get remote.origin.url`, so anything you track while inside that
checkout is filed against the project without naming it. The `name` is what you pass to `--project`.

## Per repository overrides

A `.workingon.yaml` beside a checkout is merged over your global config, so it only needs the keys it changes:

```yaml
settings:
  toggl_wid: 1234567
mappings:
  - name: "THIS-REPO"
    toggl_pid: 987654
```

It is looked for from the working directory upwards, stopping at the repository root. Credentials are ignored
there - a checked in overlay cannot change which account you authenticate as.
