---
name: wo
description: Track time in Toggl Track from the command line with the `wo` tool - start and stop timers, see what is running, review or tidy a day. Use when the user says they are starting or finishing a piece of work, asks what they are working on or how long they have been at it, wants a timer stopped or switched, asks what they did today or yesterday, or wants a day's ragged entries tidied up. Also use when settling into work in a repository, to check whether that checkout tracks time and offer to start a timer if none is running.
---

# wo

`wo` books time to Toggl Track. Every command that prints anything takes
`--json` - **always pass it**, and parse the document rather than the prose.

## The two rules

**Run it from the user's repository.** `wo` picks the project from the
`.workingon.yaml` at the root of the checkout you are standing in. Run it from
the working directory of whatever the user is working on; running it from
somewhere else books the time to the wrong project, or to none.

**`--json` never prompts.** It implies non-interactive: anything `wo` would
have asked about becomes an error instead of a question. So a workspace that
requires a task needs `--task <id>` on the command line - the error says so
when it happens.

## Offering to start a timer

When the user begins working on something in a checkout that tracks time, and
nothing is running, **ask whether they want a timer started**. Two checks, in
this order, and both have to pass before you say anything:

```
wo where --json     # is this checkout set up to book time?
wo now --json       # is a timer already running?
```

**Only offer where `configured` is `true`.** That field is the whole gate. It
means a `.workingon.yaml` was found - `wo` walks up from the working directory
to the repository root looking for one, so a subdirectory of a tracked checkout
counts and an unrelated repository above it does not.

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
nothing at all** - no offer, no suggestion to run `wo init`, no mention that
time tracking exists. Most repositories are not tracked, and a prompt in every
one of them is noise. Only bring it up if the user raises it themselves.

Where it is `true` and `wo now --json` reports `running: false`, offer once -
naming the project from `wo where`, so the user knows where it would land:

> Want me to start a timer for Learning Platform Development?

Offer once per session, not on every message. If they decline, or if they are
plainly doing something other than the work the repository is for, drop it. If
a timer is already running, do not offer - mention what it is only if the user
seems to have moved on to something else.

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
| What is running | `wo now --json` | `{running, entry}` - `entry` is `null` when nothing runs |
| Start a timer | `wo start "<description>" --json` | `{action: "started", entry}` |
| Stop it | `wo stop --json` | `{action: "stopped", entry}` |
| Carry on with the last thing | `wo continue --json` | `{action: "continued", entry}` |
| Book a finished stretch | `wo add "<description>" <start> <duration> --json` | `{action: "added", entry}` |
| A day's entries | `wo show [date] --json` | `{date, entries, total_seconds}` |
| Projects | `wo projects --json` | `{projects, current_project}` |
| Tasks for this project | `wo tasks --json` | `{tasks, project, hidden_archived}` |
| Saved templates | `wo templates --json` | `{templates}` |
| Tidy a day | `wo sanitize [date] --json` | `{date, adjustments, saved}` |

`[date]` is `today`, `yesterday`, a weekday name for the most recent such day,
or a date in the user's configured layout.

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

`--dry` runs the whole thing and reports the entry it *would* have created
without writing to Toggl. The entry comes back with `"id": 0`. Use it when the
user asks what would happen, or to check a task resolves before committing.

Before starting something new, `wo now --json` is worth a look - if a timer is
already running for the same thing, say so instead of restarting it.

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
