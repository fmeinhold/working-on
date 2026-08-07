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

## Projects

A time entry's project is resolved in this order:

1. the project the task belongs to, for an entry that ended up with one
2. `--project`, which is either a toggl project id or the name of a project in your workspace - or `WO_TOGGL_PROJECT`
   in the environment, which is the same thing said once
3. `toggl_default_pid` - usually set per checkout in a `.workingon.yaml`

The task leads because a task belongs to exactly one project, and toggl refuses an entry that files it under
another - so where the two disagree, only the task can be honoured. An entry with no task takes the rest of the
order as written.

```
wo start "fixing the parser" --project "SW Biz Dev"
```

Names are matched exactly and case insensitively against the projects `wo projects` lists, and an active project wins
over an archived one of the same name. A `--project` that names nothing, or that two projects answer to, is an error
rather than a quiet fall back to the default - filing time under the wrong project is worse than not filing it. Name
one by id to settle it.

If none of the three produce a project, `toggl_pid_required` decides whether that is an error or an entry with no
project at all.

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
there - a checked in overlay cannot change which account you authenticate as. Note that a list, such as `templates`,
replaces the one it overrides rather than adding to it.

## Tasks

A time entry's task is resolved in this order:

1. `--task`, by id or by name - or `WO_TOGGL_TASK` in the environment, which is the same thing said once
2. the summary, when it *is* a task key or the exact name of a task in the project - `wo start "ATD Conference"`
3. `toggl_default_task`, for an entry that landed in `toggl_default_pid` - usually set per checkout in a
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
inherited from a default or from the entry being continued.

## Templates

An entry you book over and over is worth naming once. A template is addressed by its alias, and stands in for the
summary:

```yaml
templates:
  - alias: "ds"
    description: "Daily Standup"
    toggl_pid: 12345678
    toggl_task: 87654321
    start: "17:30"
    stop: "17:45"

  - alias: "call"
    description: "Call with {{.caller}}"
```

```
wo add ds
wo add call -t caller=Sam
```

`wo templates` lists what you have, so an alias you set up months ago is one command away rather than a trip to the
config file:

```
2 templates

  ds    Daily Standup          · SW Biz Dev · 05 Front End Development · 17:30-17:45
  call  Call with {{.caller}}

Book one with `wo add <alias>` or `wo start <alias>`
```

The project and task are named where they can be looked up, and shown as `#id` where they cannot - offline, or
against a project that has since been deleted. A set of templates that pins neither is listed without any lookup at
all.

The description is a [Go template](https://pkg.go.dev/text/template), filled from `-t/--templateArgs`:

```
wo add call -t caller=Sam -t topic="the roadmap"
```

A placeholder you give no argument for is asked about rather than left blank, so the flags are a way of answering in
advance rather than something to remember:

```
wo add call 9:00 30m

Template "call" asks for 2 arguments:
caller: Sam
topic: the roadmap
```

Only what was left open is asked about, and an answer left blank stays open - it renders as `<no value>`, as does
every placeholder in a run with nobody to ask, so `wo add call` still books something from a script or a cron job
rather than waiting on an answer no one will type.

`toggl_pid` and `toggl_task` pin where the entry lands, and both are optional - a template that names neither is
filed the way any other entry is, through the order under [Projects](#projects) and [Tasks](#tasks). An explicit
`--task` still overrides the task, and `--project` the project of a template that pins no task, so a template is a
default rather than a rule.

Whichever task the entry ends up with decides the project, so a template pinning a task lands in that task's own
project - `toggl_default_pid` does not pull it away, and neither does a `toggl_pid` on the template naming somewhere
the task does not live.

`start` and `stop` are times of day, `HH:MM`, and are stamped with today's date; the duration follows from the pair.
Leave them out and you give the times on the command line as usual:

```
wo add ds 9:00 1h
```

Unlike the project and the task, times a template does set are *not* overridden from the command line - `wo add ds`
books today 17:30 to 17:45 whatever times you type after it. Keep the times out of a template you want to book for
other days.

`wo start` ignores a template's times altogether, taking only its description, project and task - a timer runs from
when you start it until you stop it. So `wo start ds` begins the standup now, whatever the template says.

Aliases are matched case insensitively, and are looked up after task keys but before task names - so an alias never
shadows a real task id, and a task whose name happens to match an alias yields to it.

## Tidying a day

A day tracked as you go is ragged: entries start at 09:03:47, a note you typed while doing something else sits as
four minutes between two long stretches, and there are gaps where you carried on working without saying so.
`wo sanitize` tidies that up:

```
wo sanitize
wo sanitize yesterday
```

```
⏲  Friday, 7.8.2026 - 3 entries to tidy

 Was                   Now                    Description                  Why
--------------------- ---------------------- ---------------------------- ---------------------------
 07:46-09:54 (2h 9m)   07:45-09:55 (2h 10m)   DBQ import                   snapped
 09:54-10:58 (1h 5m)   09:55-11:00 (1h 5m)    DBQ import discussion        snapped
 11:14-11:17 (4m)      11:00-11:30 (30m)      Research into state codes    snapped, extended back

Nothing was stretched into 12:00-13:00.

Save 3 entries [y/N]:
```

Two rules decide where the time goes:

* a gap **before a short entry** - under 15 minutes - belongs to that entry, so a note grows to meet the entries
  either side of it
* **every other gap** goes to the entry that ran into it, which is what carrying on without saying so looks like

and ragged times are rounded to the nearest five minutes on the way. `--snap` and `--short` say otherwise for one
run, and `--snap 0` leaves the times exactly where they are.

Nothing is created and nothing is deleted - only the start and stop times of entries that are already there move.
The ends of the day are left where they are, a timer that is still running is not touched, and two entries that
already overlap are left to you. What would change is always shown first: `--dry` shows it and stops, `--yes` saves
without asking, and a run with nobody to ask changes nothing unless it was given `--yes`.

### No work zones

Hours you do not work are worth saying out loud, so that a gap over lunch stays a gap rather than being handed to
whichever entry is next to it:

```yaml
sanitize:
  no_work:
    - "12:00-13:00"
  snap: "5m"
  short: "15m"
```

An entry is never stretched into or across a zone: each side of the gap closes what it can reach and the zone stays
empty. An entry that genuinely overlaps one - a lunch and learn - is left exactly as it is, since that is time you
really did work.

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
