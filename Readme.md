# Working-On

## What it is

First and foremost it's useful to me - it's a tool to easly track my time with toggl track from the command line,
using different sources as tasks. At the moment toggl is the only source, but the source interface is there so
others can be added. It is also an excuse to learn go.

It is built to be run from inside your local repositories. Running `wo init` in a working copy maps that checkout
to a toggl project, and time you start there is filed against it from then on without you naming it - see
[In your repositories](#in-your-repositories).

It has only been tested on MacOS and Linux so far.

## Install

```
brew install fmeinhold/tap/wo
```

That pulls a prebuilt binary for macOS or Linux, on intel or arm. `brew upgrade wo` moves you on.

To build it yourself instead, with go 1.20 or later:

```
git clone https://github.com/fmeinhold/working-on.git
cd working-on
make install
```

`make install` writes to `/usr/local/bin` and asks for sudo doing it; `PREFIX=~/.local make install SUDO=`
puts it somewhere that does not.

`wo version` tells you which build you are on.

## Completion

The homebrew install sets up completion for bash, zsh and fish on its own - the cask runs the binary it just
installed, so the completions describe the version you actually have.

Built from source, `wo completion <shell>` writes the script and you put it where your shell looks:

```
wo completion fish > ~/.config/fish/completions/wo.fish
wo completion zsh > "${fpath[1]}/_wo"
wo completion bash > /usr/local/etc/bash_completion.d/wo
```

`wo completion --help` covers the rest, powershell included. It completes commands and flags, and needs no config
to do it, so it works before `wo init` has run.


## Setup

Run ```wo init``` - this will ask you for your toggl track api key (https://track.toggl.com/profile) and some other,
mostly date and time related questions. It checks the token before going any further, and lets you pick a workspace
and a default project from what the account actually has. The config is written to ~/.config/working_on/config.yaml.

The date and time questions are answered for you from your locale - `LC_ALL`, `LC_TIME` or `LANG`, and on macOS the
system setting where none of those are exported. A machine set to `en_US` is offered `1/2/2006` and a twelve hour
clock, one set to `de_DE` gets `2.1.2006` and a twenty four hour one, and a locale that names somewhere unknown -
or names nowhere at all - falls back to the US pair. It is only ever the default the prompt shows, so type over it
where it guessed wrong.

Listings of projects and tasks show the first 20; typing part of a name narrows them to what matches, and `*` brings
the whole list back, so a workspace with hundreds of projects is still one you can pick from.

If you would rather write it yourself, copy `config.example.yaml` there instead - it documents every setting.

To avoid the token appearing in your terminal scrollback:

```
wo init --token "$(pbpaste)"
```

### In your repositories

`wo` is meant to be run from inside the checkout you are working in. That is how it knows what you are working on
without being told twice: the repository you are sitting in names the toggl project, so `wo start "fixing the
parser"` is the whole command rather than a `--project` typed out again every time.

So run `wo init` a second time, from inside each repository you track time against. It sets that checkout up rather
than your account: it asks which toggl project and task the work done there belongs to, and writes a
`.workingon.yaml` at the repository root recording the answer.

```
cd ~/Source/some-project
wo init
```

`--global` and `--local` pick which of the two files is written when the guess is wrong. A repository you never run
it in is not broken - entries from there just fall back to the global default project.

`wo where` answers which of the two files applies where you are standing, and where an entry started there would
land:

```
$ wo where
Directory      /Users/felix/Source/some-project/lib
Repository     /Users/felix/Source/some-project
Config         /Users/felix/.config/working_on/config.yaml
Project        Learning Platform Development (188362780)
```

The overlay is looked for from the working directory upwards, so a subdirectory of a checkout answers with the
checkout. It needs no config of its own, so it still runs - and still explains itself - in a repository that has
never been set up.

`--show` adds the settings those two files come to once merged, which is what the commands actually run on and is
not what either file says on its own. The tidying settings are the ones `wo sanitize` would use, so a value left
out reads as the default standing in for it rather than as a blank:

```
$ wo where --show
Directory      /Users/felix/Source/some-project
Repository     /Users/felix/Source/some-project
Config         /Users/felix/.config/working_on/config.yaml
Project        Learning Platform Development (188362780)

Settings
  location                   America/New_York
  day_first                  false
  date_layout                1/2/2006
  date_time_layout           1/2/2006 03:04pm
  toggl_api_token            set
  toggl_wid                  1562374
  toggl_pid_required         true
  toggl_task_required        true
  toggl_default_pid          188362780
  toggl_default_task         87708632
  toggl_default_description  not set

Sanitize
  snap                       5m
  short                      15m
  no_work                    12:00-13:00
  day_ends                   17:30

Templates      ds, call
```

Your API token is never printed, only whether there is one - this is output that ends up pasted into bug reports.
A setting that cannot be read fails here the same way it would fail the command that uses it, naming the setting
rather than showing a half-parsed version of it.

## JSON

Every command that prints anything takes `--json` and answers with one document instead:

```
$ wo now --json
{
  "running": true,
  "entry": {
    "id": 4510033242,
    "description": "DBQ import",
    "project": { "id": 188362780, "name": "Learning Platform Development" },
    "task": { "id": 87708632, "name": "05 Front End Development" },
    "start": "2026-08-07T07:45:00-04:00",
    "seconds": 7800,
    "running": true,
    "workspace_id": 1562374
  }
}
```

Times are RFC 3339 in your configured zone, so the offset records where the day was worked. Lengths are in seconds -
the one form that needs no parsing and cannot be read two ways - and for a running timer it is how far it has got.
A `project` or `task` is `null` where the entry has none, and its `name` is left out where the id could not be
looked up, so the id always stands on its own.

A failure is a document too, on stderr, with nothing at all on stdout - so whatever arrives on stdout can be parsed
without checking whether it is an error first:

```
$ wo stop --json
{
  "error": "no time entry is currently running"
}
```

`--json` also means nobody is there to be asked. Anything `wo` would have put as a question - which task, what to
call an entry that has no description - is an error instead, so it never sits waiting on an answer that is not
coming. Name the task with `--task` where the workspace requires one, and `wo sanitize --json` saves only with
`--yes`, reporting which it did as `saved`.

## Claude Code

`wo skill` prints a [Claude Code](https://claude.com/claude-code) skill built on the JSON above, for starting and
stopping timers from an editor session. `--install` puts it where Claude Code looks:

```
wo skill --install
```

That writes `~/.claude/skills/wo/SKILL.md` - or the same path under `$CLAUDE_CONFIG_DIR` where you have set one,
and `--dir` says somewhere else outright. The skill is embedded in the binary, so a homebrew install can write it
without a checkout to copy from. A skill already there is left alone unless `--force` says otherwise, so a run of
this never quietly discards one you have edited - including the case where you symlinked a checkout into place and
overwriting would edit the checkout.

It gates itself on `wo where`: it offers to start a timer in a checkout that has a `.workingon.yaml` and stays
quiet in every other one, on the grounds that most repositories are not ones you book time against.

Without `--install` it goes to stdout, which is the version to read before letting an agent act on it.

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

### Ignoring it globally

The file maps your checkout to your toggl account, and those project ids mean nothing in anyone else's. It belongs
in your own global ignore rather than in each repository's `.gitignore` - a repository you do not own is not one you
can add it to anyway, and doing it globally is done once for every repository you will ever run `wo init` in.

Check first whether git has already been pointed at an ignore file of your own:

```
git config --global core.excludesfile
```

If that prints a path - something like `~/.gitignore_global` - append to that file, since setting it means git stops
reading anywhere else:

```
echo '.workingon.yaml' >> ~/.gitignore_global
```

If it prints nothing, git reads `~/.config/git/ignore` without being told to, so writing that file is the whole job:

```
mkdir -p ~/.config/git
echo '.workingon.yaml' >> ~/.config/git/ignore
```

A repository that would rather have the file committed - a project where everyone books to the same toggl project -
can still have it: `git add -f .workingon.yaml` once is enough, since an ignore file is only consulted about files
git is not already tracking. A repository `.gitignore` outranks your global one, so `!.workingon.yaml` there undoes
it for that checkout for good.

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

## Picking up where you left off

`wo continue` starts a fresh timer carrying the last entry's description, project and task. The block it copies
keeps its own record - this does not reopen it.

`--recent` offers the last ten things you worked on instead, newest first, and starts the one you pick:

```
$ wo continue --recent

The last 10 things you worked on:
  1) LP2 legacy DB dump for re-import · Learning Platform Development · 05 Front End Development  (8/19/2026 08:10am)
  2) Mailpit for local email dev · LaunchCycle 3.0 · 05 Front End Development  (8/18/2026 08:10am)
  3) LP3-4: DBQ OAuth app + Basil role groups · Learning Platform Development · 05 Front End…  (8/17/2026 04:15pm)
  ...
Which one: 2
```

The listing looks back 30 days, folds the same work booked over and over into one line - same description, project
and task - and leaves out a timer that is still running, since there is nothing to continue about work that has not
stopped.

A query narrows it, matching letters in order rather than as a run, so it is the shape of the thing you want rather
than its spelling:

```
wo continue --recent oauth
wo continue --recent dbqoauth      # finds "LP3-4: DBQ OAuth app"
wo continue --recent "lp3 dbq"
```

Letters that run together score above scattered ones and a word start above the middle of a word, so the obvious
answer comes first; ties keep their recency, so this morning beats three weeks ago. Typing at the `Which one`
prompt narrows the same way, and `*` brings the whole list back.

With nobody to ask - a script, or `--json` - a query that leaves exactly one entry starts it, and anything else is
an error naming what it found:

```
$ wo continue --recent import --json
{"error": "3 recent entries match \"import\" - narrow it down: \"DBQ import\", \"LP2 legacy DB dump for re-import\", \"Importer retry\""}
```

## Changing an entry

`wo modify` edits an entry that is already there. With no `--id` it means the timer running now, or the last entry
there was where nothing is running - which between them cover the two things you notice straight away: that you
started an hour ago and forgot, and that you filed the last stretch under the wrong project.

```
$ wo modify --stop 17:00
Modified  "DBQ import"
  stop     still running -> 05:00pm
  length   running -> 8h 15m
```

What you leave out is left alone. Only the fields that moved are printed, so the line you are checking is not
buried under the ones that did not.

```
wo modify --start 9:00 --stop 10:30
wo modify --project "LaunchCycle 3.0" --task "05 Front End Development"
wo modify -m "LP3-412: importer retry"
wo modify --id 4520482208 --stop 17:30
```

Ids come from the `wo show <date> -l` listing, which leads each row with one, or from `wo show --json`.

A time is a time of day on the day the entry belongs to, so `--stop 17:00` needs no date even when you are fixing
yesterday. Give one - `"yesterday 9:00"`, a weekday, or a date in your configured layout - where the entry runs
somewhere else. A `--stop` that falls before the start is read as the following morning, which is what a shift over
midnight is, and one that says its own date is taken at its word and refused if it runs backwards.

Three rules worth knowing, because they are choices rather than consequences:

- **Moving a start leaves the stop where it was.** The entry gets longer or shorter rather than sliding along the
  day: "start it at nine" is a statement about when work began.
- **A `--stop` on a running timer ends it there.** That is how a day you forgot to stop gets its evening back.
- **A task cannot follow the entry to another project.** Change the project without naming a task and the entry is
  left without one, which is said out loud rather than done quietly.

`--dry` shows the change and stops, the same as everywhere else. `--json` answers with the entry as it now stands,
the one it replaced beside it, and a list of what moved:

```
$ wo modify --stop 17:00 --json
{
  "action": "modified",
  "entry": { "id": 4510033242, "start": "...", "stop": "...", "seconds": 29700, "running": false },
  "was":   { "id": 4510033242, "start": "...", "seconds": 8610, "running": true },
  "changed": ["stopped"],
  "saved": true
}
```

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
The ends of the day are left where they are, and two entries that already overlap are left to you. A timer that is
still running is not touched either, unless it has outlived the day it started in and you have said when that is -
see [the end of the day](#the-end-of-the-day). What would change is always shown first: `--dry` shows it and stops, `--yes` saves
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

### The end of the day

A timer you forget to stop keeps running: you close the laptop on friday evening and monday morning finds a single
entry seventeen hours long. `day_ends` says when your day is over, and an entry that outlived it is cut back there:

```yaml
sanitize:
  day_ends: "18:00"
```

```
 Was                    Now                  Description   Why
---------------------- -------------------- ------------- ----------------------------
 17:03-09:12 (16h 9m)   17:05-18:00 (55m)    DBQ import    stopped at end of day, snapped
```

It applies to the timer that is *still* going, which is the state you usually find these in - that entry is ended
at the end of its day rather than left alone, the one case where sanitize stops a running timer. It is otherwise
the same reviewed change as any other: shown first, saved only once you say so.

The day is the one the entry *started* on, so the entry above belongs to `wo sanitize yesterday` even though it was
still running this morning. `--day-ends` says it for one run without touching your config.

Nothing is capped unless you set it - an entry is only ever shortened by a time you gave. Note that this is a guess
by nature: it says you worked until six because that is when you usually stop, not because anything recorded it. An
entry that *began* after the day ended is left alone, since there is no honest end to give it.

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

## Releasing

Tagging is the whole release process - the version lives in the tag and nowhere else, so there is no file to
bump before cutting one:

```
make release-patch     # v0.3.1 -> v0.3.2
make release-minor     # v0.3.1 -> v0.4.0
make release-major     # v0.3.1 -> v1.0.0
make release VERSION=v1.2.3
```

Each works out the next version from the highest tag there has ever been - not the nearest one behind HEAD,
which is a different question once a fix has been tagged on an older branch - and then refuses to go on unless
the tree is clean, you are on `main`, `main` is in step with the remote, the tests pass, and the tag is not
already there. It shows what it is about to cut and asks before pushing:

```
$ make release-patch
Releasing v0.3.1 -> v0.3.2 from 8fc89e7 on main
Testing...
Tag and push v0.3.2? [y/N]
```

`make version` prints what the last release was. `YES=1` answers the prompt for an unattended run, `SKIP_TESTS=1`
leaves the tests to CI, and `ANY_BRANCH=1`, `BRANCH=` and `REMOTE=` cover releasing from somewhere other than
`main` on `origin`. The work is in `scripts/release.sh`, which runs on its own just as well.

By hand it is two commands, and the workflow cannot tell the difference:

```
git tag -a v0.2.0 -m "v0.2.0"
git push origin v0.2.0
```

The `release` workflow runs the tests, then [GoReleaser](https://goreleaser.com) cross compiles darwin and
linux on amd64 and arm64, publishes a GitHub release with the archives and their checksums, and commits the
updated cask to [fmeinhold/homebrew-tap](https://github.com/fmeinhold/homebrew-tap). Tags are read as
semver, so `v0.2.0-rc1` and its like are marked as prereleases and homebrew leaves them alone.

Pushing to a second repository needs a token of its own - the workflow's built in `GITHUB_TOKEN` only reaches
this one. Create a fine grained personal access token with `contents: read and write` on `homebrew-tap`, and
store it here as the `HOMEBREW_TAP_TOKEN` repository secret.

To see what a release would produce without publishing anything:

```
goreleaser release --snapshot --clean --skip=publish
```
