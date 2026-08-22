package cmd

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"
)

// jsonOutput is set by the root command's --json flag.
//
// It is a package variable rather than something threaded through every
// command because two of the questions it answers - whether there is anybody
// to prompt, and how to report an error - are asked from places that never see
// a *cobra.Command.
var jsonOutput bool

// Indented, because these get read by people at least as often as by programs,
// and a single document per command means the whitespace costs nothing to
// parse.
func emit(value any) error {
	return encodeTo(os.Stdout, value)
}

// emitError reports a failure in the same form as a success, on stderr where
// an error belongs. Stdout then carries the answer or nothing at all, so a
// caller that got anything at all can parse it without looking first.
func emitError(err error) {
	_ = encodeTo(os.Stderr, struct {
		Error string `json:"error"`
	}{err.Error()})
}

func encodeTo(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")

	// Descriptions are prose, and a project called "Bread & Butter" should not
	// come back as "Bread & Butter" for want of an HTML document nobody is
	// writing.
	encoder.SetEscapeHTML(false)

	return encoder.Encode(value)
}

type ref struct {
	Id   int    `json:"id"`
	Name string `json:"name,omitempty"`
}

// named pairs an id with its name, dropping the "#id" stand-in the human
// output falls back to when a lookup fails - the id is already a field of its
// own here, and repeating it as a name would read as a project genuinely
// called "#123".
func named(id int, name string) *ref {
	if id == 0 && name == "" {
		return nil
	}

	if strings.HasPrefix(name, "#") {
		name = ""
	}

	return &ref{Id: id, Name: name}
}

// entryJSON is a time entry as --json describes it.
//
// Times are RFC 3339 in the configured zone, so the offset records where the
// day was worked rather than leaving it to be guessed. The length is given in
// seconds, which needs no parsing and cannot be read two ways - for a running
// timer it is how long it has gone so far.
type entryJSON struct {
	Id          int      `json:"id"`
	Description string   `json:"description"`
	Project     *ref     `json:"project"`
	Task        *ref     `json:"task"`
	Start       string   `json:"start,omitempty"`
	Stop        string   `json:"stop,omitempty"`
	Seconds     int64    `json:"seconds"`
	Running     bool     `json:"running"`
	Tags        []string `json:"tags,omitempty"`
	WorkspaceId int      `json:"workspace_id,omitempty"`
}

// entryOf describes one entry, naming its project and task through the same
// lookup the human output uses.
func entryOf(entry *toggl.TimeEntry, cfg *workingon.Config, names entryNames) *entryJSON {
	if entry == nil {
		return nil
	}

	loc := &cfg.Settings.Location

	view := &entryJSON{
		Id:          entry.Id,
		Description: entry.Description,
		Project:     named(entry.ProjectId, names.project),
		Task:        named(entry.TaskId, names.task),
		Seconds:     int64(entryDuration(entry) / time.Second),
		Running:     entry.IsRunning(),
		Tags:        entry.Tags,
		WorkspaceId: entry.WorkspaceId,
	}

	if entry.Start != nil {
		view.Start = entry.Start.In(loc).Format(time.RFC3339)
	}
	if entry.Stop != nil {
		view.Stop = entry.Stop.In(loc).Format(time.RFC3339)
	}

	return view
}

// For the commands that answer with a single entry and have no listing to
// share a lookup with.
func entryWith(entry *toggl.TimeEntry, cfg *workingon.Config) *entryJSON {
	if entry == nil {
		return nil
	}

	return entryOf(entry, cfg, nameResolver(entry))
}

// The total is added up here rather than left to the caller, since summing the
// entries is the first thing anyone does with this and getting a running timer
// right in that sum is the one part worth doing once.
func emitDay(day time.Time, entries []toggl.TimeEntry, cfg *workingon.Config,
	resolve func(*toggl.TimeEntry) entryNames) error {

	views := make([]*entryJSON, 0, len(entries))
	var total int64

	for i := range entries {
		view := entryOf(&entries[i], cfg, resolve(&entries[i]))
		total += view.Seconds
		views = append(views, view)
	}

	return emit(struct {
		Date         string       `json:"date"`
		Entries      []*entryJSON `json:"entries"`
		TotalSeconds int64        `json:"total_seconds"`
	}{day.Format("2006-01-02"), views, total})
}

// spanJSON is where an entry runs between, for the before and after of a
// tidying.
type spanJSON struct {
	Start   string `json:"start"`
	Stop    string `json:"stop"`
	Seconds int64  `json:"seconds"`
}

func spanJSONOf(start, stop time.Time, loc *time.Location) *spanJSON {
	return &spanJSON{
		Start:   start.In(loc).Format(time.RFC3339),
		Stop:    stop.In(loc).Format(time.RFC3339),
		Seconds: int64(stop.Sub(start) / time.Second),
	}
}

func emitSanitizePlan(day time.Time, plan []workingon.Adjustment, cfg *workingon.Config,
	resolve func(*toggl.TimeEntry) entryNames, saved bool) error {

	loc := &cfg.Settings.Location

	type adjustmentJSON struct {
		Entry *entryJSON `json:"entry"`
		Was   *spanJSON  `json:"was"`
		Now   *spanJSON  `json:"now"`
		Why   []string   `json:"why"`
	}

	adjustments := make([]adjustmentJSON, 0, len(plan))

	for _, adjustment := range plan {
		entry := adjustment.Entry

		adjustments = append(adjustments, adjustmentJSON{
			Entry: entryOf(&entry, cfg, resolve(&entry)),
			Was:   spanJSONOf(*entry.Start, entry.Start.Add(entryDuration(&entry)), loc),
			Now:   spanJSONOf(adjustment.Start, adjustment.Stop, loc),
			Why:   adjustment.Notes,
		})
	}

	return emit(struct {
		Date        string           `json:"date"`
		Adjustments []adjustmentJSON `json:"adjustments"`
		Saved       bool             `json:"saved"`
	}{day.Format("2006-01-02"), adjustments, saved})
}

// The flag is there so that "nothing is running" is a fact to be read rather
// than a null to be told apart from a lookup that failed.
func emitCurrent(entry *toggl.TimeEntry, cfg *workingon.Config) error {
	return emit(struct {
		Running bool       `json:"running"`
		Entry   *entryJSON `json:"entry"`
	}{entry != nil, entryWith(entry, cfg)})
}

// The verb is carried in the document rather than left to the caller to
// remember which command they ran, so a log of these reads on its own.
func emitEntry(action string, entry *toggl.TimeEntry, cfg *workingon.Config) error {
	return emit(struct {
		Action string     `json:"action"`
		Entry  *entryJSON `json:"entry"`
	}{action, entryWith(entry, cfg)})
}

// emitWeek is the week a day at a time, for a program reading the output.
// Every day of the week is there, including the ones with nothing on them.
func emitWeek(week []dayTotal) error {
	type dayJSON struct {
		Date     string   `json:"date"`
		Weekday  string   `json:"weekday"`
		Entries  int      `json:"entries"`
		Seconds  int64    `json:"seconds"`
		Projects []string `json:"projects"`
		Running  bool     `json:"running"`
	}

	days := make([]dayJSON, 0, len(week))
	var total int64

	for _, day := range week {
		seconds := int64(day.Tracked / time.Second)
		total += seconds

		projects := day.Projects
		if projects == nil {
			projects = []string{}
		}

		days = append(days, dayJSON{
			Date:     day.Day.Format("2006-01-02"),
			Weekday:  day.Day.Format("Monday"),
			Entries:  day.Entries,
			Seconds:  seconds,
			Projects: projects,
			Running:  day.Running,
		})
	}

	return emit(struct {
		From         string    `json:"from"`
		To           string    `json:"to"`
		Days         []dayJSON `json:"days"`
		TotalSeconds int64     `json:"total_seconds"`
	}{
		From:         week[0].Day.Format("2006-01-02"),
		To:           week[len(week)-1].Day.Format("2006-01-02"),
		Days:         days,
		TotalSeconds: total,
	})
}
