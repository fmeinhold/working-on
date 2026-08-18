---
name: wo
description: Track time in Toggl Track from the command line with the `wo` tool - start and stop timers, see what is running, review or tidy a day. Use when the user says they are starting or finishing a piece of work, asks what they are working on or how long they have been at it, wants a timer stopped or switched, asks what they did today or yesterday, or wants a day's ragged entries tidied up. Also use when settling into work in a repository, to check whether that checkout tracks time and, where it does, start an entry for the work at hand.
---

# wo

`wo` books time to Toggl Track. Every command that prints anything takes
`--json` - **always pass it**, and parse the document rather than the prose.

Every command, argument and flag is written down below. **Do not run
`wo --help` or `wo <command> --help`** - it costs a round trip to learn what
this page already says.

## The two rules

**Run it from the user's repository.** `wo` picks the project from the
`.workingon.yaml` at the root of the checkout you are standing in. Run it from
the working directory of whatever the user is working on; running it from
somewhere else books the time to the wrong project, or to none.

**`--json` never prompts.** It implies non-interactive: anything `wo` would
have asked about becomes an error instead of a question. So a workspace that
requires a task needs `--task <id>` on the command line - the error says so
when it happens.

## Every piece of work gets an entry

Where the checkout tracks time, **new work always means a new entry** - never a
question of whether to book it, only of what to book it as. Start one at the
beginning of a session, again when a session resumes after a break or on a new
day, and again whenever the work moves to a different ticket or topic. Two
checks first, in this order:

```
wo where --json     # is this checkout set up to book time?
wo now --json       # is a timer already running?
```

**This only applies where `configured` is `true`.** That field is the whole
gate. It means a `.workingon.yaml` was found - `wo` walks up from the working
directory to the repository root looking for one, so a subdirectory of a
tracked checkout counts and an unrelated repository above it does not.

`wo where` needs no config of its own and does not fail in a checkout that has
none, so it is always safe to ask.

```json
{
  "directory": "/Users/felix/Source/some-project/lib",
  "local_config": "/Users/felix/Source/some-project/.workingon.yaml",
  "project": { "id": 188362780, "name": "Learning Platform Development" },
  "configured": true
}
```

`configured: false` means this checkout books nothing in particular. **Say
nothing at all** - no entry, no offer, no suggestion to run `wo init`, no
mention that time tracking exists. Most repositories are not tracked, and a
prompt in every one of them is noise. Only bring it up if the user raises it
themselves.

Where it is `true`, what `wo now` says decides the move:

- **Nothing running.** Start an entry for the work at hand and say what it
  booked, naming the project from `wo where` so the user knows where it landed.
- **A timer already running for this same work.** Leave it alone. Say what is
  running rather than restarting it - a restart splits one stretch in two for
  no gain.
- **A timer running for something else.** The work has moved, so this is new
  work: start a fresh entry for it. `wo start` saves what was running, so it is
  one command, not two.

Describe the entry from the work itself, and **prefix the description with the
ticket number** where there is one - `"LP3-412: fix the importer retry"`.
Entries in other checkouts are somebody else's business: different repositories
book to different projects and are meant to run in parallel, so never stop one
to start another.

## Reading the result

Success: one JSON document on stdout, exit 0.

Failure: exit non-zero, `{"error": "..."}` on **stderr**, and **stdout is
empty**. So anything on stdout can be parsed without checking first, and the
error is worth reading aloud to the user - the messages are written for people.

```
$ wo stop --json          # nothing was running
{
  "error": "no time entry is currently running"
}
```

That particular error is the ordinary "nothing to stop" case, not a fault.

## Commands

| What | Command | Answers with |
|---|---|---|
| Is this checkout tracked | `wo where --json` | `{directory, local_config, project, configured}` |
| What the settings come to | `wo where --show --json` | the same, plus `config` |
| What is running | `wo now --json` | `{running, entry}` - `entry` is `null` when nothing runs |
| Start a timer | `wo start "<description>" --json` | `{action: "started", entry}` |
| Stop it | `wo stop --json` | `{action: "stopped", entry}` |
| Carry on with the last thing | `wo continue --json` | `{action: "continued", entry}` |
| Book a finished stretch | `wo add "<description>" <start> <duration> --json` | `{action: "added", entry}` |
| Change one that is there | `wo modify <flags> --json` | `{action: "modified", entry, was, changed, saved}` |
| A day's entries | `wo show [date] --json` | `{date, entries, total_seconds}` |
| Projects | `wo projects --json` | `{projects, current_project}` |
| Tasks for this project | `wo tasks --json` | `{tasks, project, hidden_archived}` |
| Saved templates | `wo templates --json` | `{templates}` |
| Tidy a day | `wo sanitize [date] --json` | `{date, adjustments, saved}` |

`[date]` is `today`, `yesterday`, a weekday name for the most recent such day,
or a date in the user's configured layout.

### What goes on the command line

`start` and `add` take a first argument saying what the entry is about - a
description, a task by name or by id, or a template alias. A task name is
matched within the project the entry lands in, so a description is never
mistaken for one.

Everything after that is times, and they may come **in any order**. Each
argument is tried as a time of day, then a range, then a duration, then a date;
whatever matches none of those joins the description.

| Looks like | Means |
|---|---|
| `9:00` | start at that time of day |
| `9:00-10:30` | start and duration in one |
| `1h30m`, `90m`, `2h` | a duration (Go's syntax - `h`, `m`, `s`) |
| `today`, `yesterday` | the date |
| `mon` … `sunday`, `monday` … | the most recent such weekday |
| `6.8`, `6.8.2026` | a date in the configured layout, shorter prefixes allowed |
| anything else | part of the description |

That last row is the trap: **keep the description in one quoted argument.**
`wo start "sync" mon` books Monday, not a description of "sync mon".

`start` with no time starts now. `add` needs a duration one way or another -
`9:00 1h`, `9:00-10:00`, or `9:00 --stop 10:00`.

### Flags

`--json` is global and works on every command.

| Command | Flags |
|---|---|
| `start` | `-a, --append` · `-c, --continue` · `-d, --dry` · `-p, --project <id\|name>` · `--task <id\|name>` · `--pick-task` · `-t, --templateArgs k=v` · `-w, --wid <id>` |
| `add` | the same, plus `-s, --stop <time>` and `-f, --fuzzy`; no `--continue` |
| `modify` | `--id <n>` · `--start <time>` · `-s, --stop <time>` · `-p, --project <id\|name>` · `--task <id\|name>` · `-m, --description <text>` · `-d, --dry` |
| `continue` | `-d, --dry` · `-a, --append` · `--pick-task` |
| `where` | `-s, --show` |
| `stop`, `now`, `templates` | none worth passing (`now` also has `-p, --prompt`, for shell prompts) |
| `show [date]` | `-l, --list` · `--from <hour>` · `--to <hour>` |
| `sanitize [date]` | `-d, --dry` · `-y, --yes` · `--snap <duration>` · `--short <duration>` · `--day-ends <time>` |
| `projects` | `-a, --archived` |
| `tasks` | `-r, --refresh` · `-a, --all` · `--archived` |

What the less obvious ones do:

- `--append` back-dates the start to where the last entry stopped, so the gap
  since then belongs to this entry.
- `--continue` (or the `continue` command) starts a fresh entry carrying the
  last one's description, project and task, and takes no summary of its own.
- `--dry` does the whole thing and reports the entry it *would* have created,
  coming back with `"id": 0`. Nothing reaches Toggl.
- `--pick-task` **prompts**, so it is useless under `--json` - pass `--task`
  instead. Same for anything else that would ask a question.
- `--show` on `where` adds a `config` object: the global file and the checkout's
  overlay merged, which is what the commands run on and is not what either file
  says alone. `config.sanitize` holds the values tidying would use, defaults
  filled in, so it answers "what would `wo sanitize` do here" without running
  it. The API token is never in there - `toggl_api_token_set` is a bool.
- `--snap` is the grid times round to, `0` to leave them alone (default `5m`);
  `--short` is the length under which an entry is a stub that takes the gaps
  around it (default `15m`); `--day-ends` is the time work stops, as `"18:00"`.
  All three override the config for one run.

An entry looks like this. `seconds` is elapsed-so-far while `running` is true,
and there is no `stop` until it ends:

```json
{
  "id": 4510033242,
  "description": "DBQ import",
  "project": { "id": 188362780, "name": "Learning Platform Development" },
  "task": { "id": 87708632, "name": "05 Front End Development" },
  "start": "2026-08-07T07:45:00-04:00",
  "stop": "2026-08-07T09:55:00-04:00",
  "seconds": 7800,
  "running": false,
  "workspace_id": 1562374
}
```

`project` and `task` are `null` where the entry has none. A `name` is absent
where the id could not be looked up - the `id` is always there.

## Starting and stopping

Starting a timer saves whatever was already running, so switching tasks is one
command, not two:

```
wo start "reviewing the parser PR" --json
```

Say what it booked, using the resolved project and task names rather than ids.
If the user names a task, pass it: `--task <id>` by id, or `--task "<name>"`
by name within the project. `wo tasks --json` lists what is available.

Reach for `--dry` when the user asks what would happen, or to check a task
resolves before committing to it.

Before starting something new, `wo now --json` is worth a look - if a timer is
already running for the same thing, say so instead of restarting it.

## Changing an entry

`wo modify` edits an entry that is already there. With no `--id` it is about
the timer running now, or the last entry there was where nothing is running.
`--id` names any other entry - `wo show [date] --json` has the ids, and the
human listing does not print them.

```
wo modify --stop 17:00 --json
wo modify --id 4520482208 --start 9:00 --project "LaunchCycle 3.0" --json
```

**What you leave out is left alone**, so name only what changes. Times are a
time of day on the day the entry belongs to - `--stop 17:00` needs no date even
for yesterday's entry - and a `--stop` before the start is read as the next
morning. Give a date (`--stop "yesterday 17:00"`) to mean another day.

Three behaviours to state when you report what you did, because they are
choices the user cannot see from the command:

- Moving a start leaves the stop where it was, so the entry changes length
  rather than sliding along the day.
- A `--stop` on a running timer ends it there.
- Changing the project without naming a task leaves the entry with no task: a
  task belongs to the project it was made in. `changed` says so.

`--dry` shows the change without saving, and `saved` in the document says which
happened. **These are hours the user already worked** - never modify a past
entry unless they asked for that change, and read the `was` and `entry` pair
back to them rather than reporting "done".

## Tidying a day

`wo sanitize --json` **shows without saving** unless given `--yes`, which is
the safe default and should stay that way:

1. Run it without `--yes` and show the user what would change - each
   adjustment carries `was`, `now`, and `why`.
2. Only on their say-so, run it again with `--yes`.

`saved` in the document says which of the two happened. Never pass `--yes` on
the first run: it rewrites real time entries, and the user should see the plan
first.

## When it is not set up

Two different situations, and only one of them is worth raising:

**No global config** - an error saying so from any command means `wo init` has
never been run on this machine. Setup is interactive, so tell the user to run
`wo init` themselves rather than trying to drive it. Worth mentioning, because
nothing will work until they do.

**No `.workingon.yaml` in this checkout** - `wo where --json` reporting
`configured: false`. This is the ordinary state of most repositories and not a
problem to solve. Stay quiet about it unless the user asks.
