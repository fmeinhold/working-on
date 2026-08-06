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

Listings of projects and tasks show the first 20; typing part of a name narrows them to what matches, and `*` brings
the whole list back, so a workspace with hundreds of projects is still one you can pick from.

If you would rather write it yourself, copy `config.example.yaml` there instead - it documents every setting.

To avoid the token appearing in your terminal scrollback:

```
wo init --token "$(pbpaste)"
```

Run `wo init` again inside a repository and it sets that checkout up instead: it asks which project and task work
done there belongs to, and writes a `.workingon.yaml` at the repository root. `--global` and `--local` pick the file
explicitly when the guess is wrong.

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

A `.workingon.yaml` beside a checkout is merged over your global config, so it only needs the keys it changes.
`wo init` writes one for you, or you can write it yourself:

```yaml
settings:
  toggl_default_pid: 987654
  toggl_default_task: 123456
```

`toggl_default_task` is what a new entry books against unless it names a task of its own or is given one with
`--task`. It applies only to entries that landed in `toggl_default_pid`, since that is the project the task belongs
to.

It is looked for from the working directory upwards, stopping at the repository root. Credentials are ignored
there - a checked in overlay cannot change which account you authenticate as. Note that a list, such as `mappings`,
replaces the one it overrides rather than adding to it.

## Tasks

A time entry's task is resolved in this order:

1. `--task`, by id or by name - or `WO_TOGGL_TASK` in the environment, which is the same thing said once
2. the summary, when it *is* a task key or the exact name of a task in the project - `wo start "ATD Conference"`
3. the `toggl_task` of the mapping this repository matches
4. `toggl_default_task`, for an entry that landed in `toggl_default_pid` - usually set per checkout in a
   `.workingon.yaml`

Some workspaces want a task on every entry. Set `toggl_task_required: true` and `wo` asks which one instead of
letting toggl refuse the entry:

```
Tasks in project 221284439:
   1) 01 Define / Discuss / Review / Updates
   2) 04 Content / Updates / Maintenace
   3) 05 Front End Development
  ... 8 more - type part of a name to narrow the list
Which task: front

1 matching "front":
  1) 05 Front End Development
  (* for the whole list)
Which task: 1
```

An entry that was never given a summary takes its name from the task chosen. Where a task is required and there is
nobody to ask - a script, a cron job - `wo` says so and creates nothing, so pass `--task` there.

`--pick-task` asks the same question where the workspace does not require one, and asks even about a task that was
inherited from a mapping, a default, or the entry being continued.

## Entries without a description

A toggl workspace can be set up to require a description, and that applies to the timer already running as much as to
the entry you are creating - starting or stopping something saves the running one. `wo` asks what to call an entry
that has none, before toggl gets the chance to refuse it:

```
The running entry has no description, and toggl needs one to save it.
What was it [Untitled]:
```

Set `toggl_default_description` to answer that question once and for all:

```yaml
settings:
  toggl_default_description: "Development"
```

A run with nobody to ask - a script, a cron job - never blocks on the question; it calls the entry `Untitled`.
