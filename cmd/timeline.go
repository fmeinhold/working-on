package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"
)

const (
	// slotStep is the height of one row of the timeline, and slotCells how many
	// characters wide that row is drawn - together they decide how fine a block
	// edge can be placed, which at these values is two and a half minutes.
	slotStep  = 30 * time.Minute
	slotCells = 12

	blockFill = "█"
)

// dayWindow is the span of the day a timeline draws, as clock time.
//
// An end that was asked for is a bound; one that was merely assumed gives way
// to whatever was tracked outside it.
type dayWindow struct {
	from      clockTime
	to        clockTime
	fixedTo   bool
	fixedFrom bool
}

// defaultWindow is a working day. It is only a starting point: anything tracked
// outside it widens the timeline rather than being left out of it.
var defaultWindow = dayWindow{from: clockTime{hour: 6}, to: clockTime{hour: 18}}

// blockColours are cycled through so that neighbouring blocks can be told apart,
// and so a block can be matched to the label beside it. color leaves them out
// when stdout is not a terminal.
var blockColours = []*color.Color{
	color.New(color.FgCyan),
	color.New(color.FgMagenta),
	color.New(color.FgGreen),
	color.New(color.FgYellow),
	color.New(color.FgBlue),
	color.New(color.FgRed),
}

// block is one entry placed on the timeline.
type block struct {
	begin  time.Time
	finish time.Time
	label  string
	colour *color.Color
}

// RenderTimeline draws the day as a column of half hour slots, each entry
// filling the time it covers.
//
// An end of the window that was only assumed is what the day is expected to
// look like rather than a filter: an entry outside it pulls the timeline out to
// meet it, since a day view that quietly hid tracked time would be worse than
// useless. An end that was asked for holds, and what it leaves out is said
// underneath.
func RenderTimeline(day time.Time, entries []toggl.TimeEntry, cfg *workingon.Config,
	window dayWindow, resolve func(*toggl.TimeEntry) entryNames) string {

	loc := &cfg.Settings.Location
	day = startOfDay(day, loc)
	heading := formatMoment(day, cfg.Settings.DateLayout)

	if len(entries) == 0 {
		return fmt.Sprintf("⏲  Nothing tracked on %s.\n", heading)
	}

	var (
		blocks     []block
		total      time.Duration
		anyRunning bool
	)

	for i := range entries {
		entry := &entries[i]
		duration := entryDuration(entry)
		total += duration
		anyRunning = anyRunning || entry.IsRunning()

		begin := entry.Start.In(loc)
		finish := begin.Add(duration)
		// An entry that ran past midnight is the previous day's as far as this
		// view is concerned; drawing it would add rows for a day nobody asked
		// about.
		if end := day.AddDate(0, 0, 1); finish.After(end) {
			finish = end
		}

		blocks = append(blocks, block{
			begin:  begin,
			finish: finish,
			label:  timelineLabel(entry, resolve(entry), duration),
			colour: blockColours[i%len(blockColours)],
		})
	}

	slots := timelineSlots(day, window, blocks)
	clock := timeLayout(cfg)

	gutter := 0
	for _, slot := range slots {
		if width := len(slot.Format(clock)); width > gutter {
			gutter = width
		}
	}

	var out strings.Builder
	fmt.Fprintf(&out, "⏲  %s\n\n", heading)

	// Continuation lines carry a label whose block starts in a slot that
	// already has one; they line up under the labels rather than the bars.
	indent := strings.Repeat(" ", gutter+slotCells+5)

	for _, slot := range slots {
		fmt.Fprintf(&out, " %*s │%s│", gutter, slot.Format(clock), renderBar(slot, blocks))

		labels := labelsStartingIn(slot, blocks)
		if len(labels) == 0 {
			out.WriteString("\n")
			continue
		}

		for i, label := range labels {
			if i > 0 {
				out.WriteString(indent)
			} else {
				out.WriteString(" ")
			}
			out.WriteString(label)
			out.WriteString("\n")
		}
	}

	fmt.Fprintf(&out, "\n %*s   %s\n", gutter, "Total", tableDuration(total))

	if note := missedNote(slots, blocks, clock); note != "" {
		out.WriteString("\n" + note)
	}

	if anyRunning {
		out.WriteString("\nA timer is still running, so the total is what it is right now.\n")
	}

	return out.String()
}

// missedNote accounts for time the drawn slots leave out, which only a --from
// or --to can cause. The total above counts the whole day, so without this the
// bars and the number under them would disagree with nothing to explain it.
func missedNote(slots []time.Time, blocks []block, layout string) string {
	if len(slots) == 0 {
		return ""
	}

	first := slots[0]
	last := slots[len(slots)-1].Add(slotStep)

	var (
		missed  time.Duration
		entries int
	)

	for _, b := range blocks {
		outside := b.finish.Sub(b.begin) - overlap(b.begin, b.finish, first, last)
		if outside <= 0 {
			continue
		}
		missed += outside
		entries++
	}

	if entries == 0 {
		return ""
	}

	return fmt.Sprintf("%s outside %s-%s is not drawn, across %s.\n",
		tableDuration(missed), first.Format(layout), last.Format(layout),
		pluralEntries(entries))
}

// overlap is how much of one span falls inside another.
func overlap(begin, finish, from, to time.Time) time.Duration {
	if begin.Before(from) {
		begin = from
	}
	if finish.After(to) {
		finish = to
	}

	if !finish.After(begin) {
		return 0
	}

	return finish.Sub(begin)
}

func pluralEntries(count int) string {
	if count == 1 {
		return "1 entry"
	}
	return fmt.Sprintf("%d entries", count)
}

// timelineSlots is every half hour the timeline draws: the window, widened at
// either end that was assumed rather than asked for to take in what falls
// outside it.
func timelineSlots(day time.Time, window dayWindow, blocks []block) []time.Time {
	first := clockOn(day, window.from)
	last := clockOn(day, window.to)

	for _, b := range blocks {
		if !window.fixedFrom && b.begin.Before(first) {
			first = b.begin
		}
		if !window.fixedTo && b.finish.After(last) {
			last = b.finish
		}
	}

	first = floorToSlot(day, first)
	last = ceilToSlot(day, last)

	if !last.After(first) {
		last = first.Add(slotStep)
	}

	var slots []time.Time
	for slot := first; slot.Before(last); slot = slot.Add(slotStep) {
		slots = append(slots, slot)
	}

	return slots
}

// renderBar draws one slot: a cell is filled by whichever block covers the
// middle of the time it stands for, and left blank when none does.
func renderBar(slot time.Time, blocks []block) string {
	cells := make([]int, slotCells)
	for cell := range cells {
		offset := time.Duration(cell)*(slotStep/slotCells) + slotStep/slotCells/2
		cells[cell] = blockAt(slot.Add(offset), blocks)
	}

	var bar strings.Builder
	for start := 0; start < len(cells); {
		end := start
		for end < len(cells) && cells[end] == cells[start] {
			end++
		}

		if cells[start] < 0 {
			bar.WriteString(strings.Repeat(" ", end-start))
		} else {
			bar.WriteString(blocks[cells[start]].colour.Sprint(strings.Repeat(blockFill, end-start)))
		}

		start = end
	}

	return bar.String()
}

// blockAt is the index of the block covering an instant, or -1 for a gap.
func blockAt(moment time.Time, blocks []block) int {
	for i, b := range blocks {
		if !moment.Before(b.begin) && moment.Before(b.finish) {
			return i
		}
	}
	return -1
}

// labelsStartingIn names the blocks that begin within a slot, each marked in
// its own colour so it can be matched to the bar it belongs to.
func labelsStartingIn(slot time.Time, blocks []block) []string {
	end := slot.Add(slotStep)

	var labels []string
	for _, b := range blocks {
		if b.begin.Before(slot) || !b.begin.Before(end) {
			continue
		}
		labels = append(labels, b.colour.Sprint(blockFill)+" "+b.label)
	}

	return labels
}

// timelineLabel describes an entry in one line, leaving out a project or task
// that would only repeat what the description already says.
func timelineLabel(entry *toggl.TimeEntry, names entryNames, duration time.Duration) string {
	parts := []string{describedAs(entry.Description)}

	for _, name := range []string{names.project, names.task} {
		if name != "" && name != entry.Description {
			parts = append(parts, name)
		}
	}

	label := strings.Join(parts, " · ")

	if entry.IsRunning() {
		return fmt.Sprintf("%s (%s, running)", label, tableDuration(duration))
	}

	return fmt.Sprintf("%s (%s)", label, tableDuration(duration))
}

// clockOn is a time of day on a given date. An hour of 24 is midnight the
// morning after, which is what a window ending at the end of the day means.
func clockOn(day time.Time, at clockTime) time.Time {
	year, month, date := day.Date()
	return time.Date(year, month, date, at.hour, at.minute, 0, 0, day.Location())
}

func floorToSlot(day time.Time, moment time.Time) time.Time {
	return moment.Add(-slotOffset(day, moment))
}

func ceilToSlot(day time.Time, moment time.Time) time.Time {
	if offset := slotOffset(day, moment); offset != 0 {
		return moment.Add(slotStep - offset)
	}
	return moment
}

// slotOffset is how far past the last slot boundary a moment falls, always
// forwards - a moment before the day starts would otherwise round the wrong way.
func slotOffset(day time.Time, moment time.Time) time.Duration {
	offset := moment.Sub(day) % slotStep
	if offset < 0 {
		offset += slotStep
	}
	return offset
}

// parseClockFlag reads a --from or --to value, accepting a bare hour as well as
// "6:00". An end of "24" is the end of the day rather than an invalid hour.
func parseClockFlag(name, value string) (clockTime, error) {
	given := strings.TrimSpace(value)

	value = given
	if !strings.Contains(value, ":") {
		value += ":00"
	}

	if value == "24:00" {
		return clockTime{hour: 24}, nil
	}

	at, matched, err := tryClock(value)
	if err != nil {
		return clockTime{}, fmt.Errorf("--%s: %q is not a valid time of day", name, given)
	}
	if !matched {
		return clockTime{}, fmt.Errorf("--%s: unable to read %q as a time of day", name, given)
	}

	return at, nil
}
