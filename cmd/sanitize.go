package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/alexeyco/simpletable"
	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
)

func NewSanitizeCommand(cfg *workingon.Config) *cobra.Command {
	var (
		dry     bool
		yes     bool
		snap    string
		short   string
		dayEnds string
	)

	sanitizeCommand := &cobra.Command{
		Use:   "sanitize [date]",
		Short: "Tidy up a day's time entries",
		Long: `Tidy a day's time entries, today unless another one is named.

Ragged times are rounded to the nearest five minutes, and the gaps between
entries are closed:

  * a gap before a short entry - under 15 minutes - is that entry's, so a note
    typed while doing something else grows to meet the entries either side
  * every other gap goes to the entry that ran into it, which is what happens
    when you carry on working without saying so

No work zones are hours nothing may be stretched into. Set them in your config
and a gap over lunch stays a gap:

  sanitize:
    no_work:
      - "12:00-13:00"

An entry that ran past midnight is asked about rather than assumed: it is the
one thing here nobody does on purpose, and only you know when you actually
stopped. day_ends is the answer on offer, so pressing enter takes it.

day_ends is the time work stops, and is what an entry that ran on is cut back
to - including a timer that is still going, which is ended there:

  sanitize:
    day_ends: "18:00"

Nothing is created and nothing is deleted - only the start and stop times of
entries that are already there move, and an entry that overlaps a no work zone
because that is when you worked is left as it is. What would change is shown
first, and saved only once you say so; --dry shows it and stops.

The date is read the way every other date in wo is: "today", "yesterday", a
weekday name for the most recent such day, or a date in your configured layout.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			day := time.Now().In(&cfg.Settings.Location)
			if len(args) > 0 {
				parsed, err := ParseDateFromArg(args[0], cfg)
				if err != nil {
					return err
				}
				day = parsed
			}

			sanitizer, err := newSanitizer(cfg, cmd, snap, short, dayEnds)
			if err != nil {
				return err
			}
			sanitizer.AskEndOfDay = endOfDayAsker(cfg)

			start := startOfDay(day, &cfg.Settings.Location)
			end := start.AddDate(0, 0, 1)

			cl := toggl.NewToggl(cfg.Settings.ToggleApiToken)
			listed, err := cl.TimeEntries.List(&start, &end)
			if err != nil {
				return err
			}

			entries := entriesStartingOn(start, listed.TimeEntries)
			plan := sanitizer.Plan(entries)

			names := &dayNames{}

			// Asked for JSON there is nobody to put the question to, so --yes
			// is the only thing that can answer it. Saying so in the document
			// beats a plan that looks applied and was not.
			if jsonOutput {
				save := len(plan) > 0 && !dry && yes
				if save {
					if err := saveSanitizePlan(cfg, plan); err != nil {
						return err
					}
				}
				return emitSanitizePlan(start, plan, cfg, names.names, save)
			}

			fmt.Print(RenderSanitizePlan(start, plan, sanitizer, cfg, names.names))

			if len(plan) == 0 || dry {
				return nil
			}

			if !confirmSanitize(yes, len(plan)) {
				return nil
			}

			return applySanitizePlan(cfg, plan)
		},
	}

	sanitizeCommand.Flags().BoolVarP(&dry, "dry", "d", false, "Show what would change and stop")
	sanitizeCommand.Flags().BoolVarP(&yes, "yes", "y", false, "Save the changes without asking")
	sanitizeCommand.Flags().StringVar(&snap, "snap", "",
		"Grid times are rounded to, 0 to leave them alone (default 5m)")
	sanitizeCommand.Flags().StringVar(&short, "short", "",
		"Length under which an entry is a stub that takes the gaps around it (default 15m)")
	sanitizeCommand.Flags().StringVar(&dayEnds, "day-ends", "",
		"Time work stops, as \"18:00\", that an entry running past it is cut back to")

	return sanitizeCommand
}

// newSanitizer reads the sanitize settings, letting the flags say otherwise for
// this one run.
func newSanitizer(cfg *workingon.Config, cmd *cobra.Command, snap, short, dayEnds string) (workingon.Sanitizer, error) {
	settings := cfg.Sanitize

	if cmd.Flags().Changed("snap") {
		settings.Snap = snap
	}
	if cmd.Flags().Changed("short") {
		settings.Short = short
	}
	if cmd.Flags().Changed("day-ends") {
		settings.DayEnds = dayEnds
	}

	return workingon.NewSanitizer(&workingon.Config{Settings: cfg.Settings, Sanitize: settings})
}

func RenderSanitizePlan(day time.Time, plan []workingon.Adjustment, sanitizer workingon.Sanitizer,
	cfg *workingon.Config, resolve func(*toggl.TimeEntry) entryNames) string {

	heading := formatMoment(day, cfg.Settings.DateLayout)

	if len(plan) == 0 {
		return fmt.Sprintf("⏲  Nothing to tidy on %s.\n", heading)
	}

	loc := &cfg.Settings.Location
	clock := timeLayout(cfg)

	table := simpletable.New()
	table.SetStyle(simpletable.StyleCompactLite)

	for _, text := range []string{"Was", "Now", "Description", "Why"} {
		table.Header.Cells = append(table.Header.Cells,
			&simpletable.Cell{Align: simpletable.AlignLeft, Text: text})
	}

	for _, adjustment := range plan {
		entry := adjustment.Entry
		names := resolve(&entry)

		values := []string{
			spanOf(entry.Start.In(loc), entryDuration(&entry), clock),
			spanOf(adjustment.Start.In(loc), adjustment.Stop.Sub(adjustment.Start), clock),
			sanitizeLabel(&entry, names),
			adjustment.Note(),
		}

		var cells []*simpletable.Cell
		for _, value := range values {
			cells = append(cells, &simpletable.Cell{Align: simpletable.AlignLeft, Text: value})
		}
		table.Body.Cells = append(table.Body.Cells, cells)
	}

	var out strings.Builder
	fmt.Fprintf(&out, "⏲  %s - %s to tidy\n\n", heading, pluralEntries(len(plan)))
	out.WriteString(table.String())
	out.WriteString("\n")

	if note := zoneNote(sanitizer.Zones); note != "" {
		out.WriteString("\n" + note)
	}

	return out.String()
}

func spanOf(begin time.Time, duration time.Duration, layout string) string {
	return fmt.Sprintf("%s-%s (%s)", begin.Format(layout),
		begin.Add(duration).Format(layout), tableDuration(duration))
}

// labelWidth is as much of an entry as a row will carry. The times are what
// this listing is about, and a full project and task path would push them off
// the side of the terminal.
const labelWidth = 60

// sanitizeLabel names the entry a row is about, adding the project or task only
// where it says something the description does not.
func sanitizeLabel(entry *toggl.TimeEntry, names entryNames) string {
	parts := []string{describedAs(entry.Description)}

	for _, name := range []string{names.project, names.task} {
		if name != "" && name != entry.Description {
			parts = append(parts, name)
		}
	}

	return shortened(strings.Join(parts, " · "), labelWidth)
}

func shortened(text string, width int) string {
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}

	return strings.TrimRight(string(runes[:width-1]), " ·") + "…"
}

func zoneNote(zones []workingon.Zone) string {
	if len(zones) == 0 {
		return ""
	}

	written := make([]string, len(zones))
	for i, zone := range zones {
		written[i] = zone.String()
	}

	return fmt.Sprintf("Nothing was stretched into %s.\n", strings.Join(written, " or "))
}

// A run with nobody to ask says what it would have done and changes nothing,
// since --yes is how a script says it meant it.
func confirmSanitize(yes bool, count int) bool {
	return confirmSanitizeWith(os.Stdin, os.Stdout, interactive(), yes, count)
}

func confirmSanitizeWith(in io.Reader, out io.Writer, interactive, yes bool, count int) bool {
	if yes {
		return true
	}

	if !interactive {
		fmt.Fprintf(out, "\nNothing was changed - run this with --yes to save %s.\n",
			pluralEntries(count))
		return false
	}

	prompt := &prompter{reader: bufio.NewReader(in), out: out}

	fmt.Fprintln(out)

	return prompt.yesNo(fmt.Sprintf("Save %s", pluralEntries(count)), false)
}

func applySanitizePlan(cfg *workingon.Config, plan []workingon.Adjustment) error {
	loc := &cfg.Settings.Location
	clock := timeLayout(cfg)

	for _, adjustment := range plan {
		saved, err := saveAdjustment(cfg, adjustment)
		if err != nil {
			return err
		}

		fmt.Printf("  %s  %s\n",
			spanOf(saved.Start.In(loc), time.Duration(saved.Duration)*time.Second, clock),
			describedAs(saved.Description))
	}

	fmt.Printf("\nTidied %s.\n", pluralEntries(len(plan)))

	return nil
}

// saveSanitizePlan writes a whole plan back without saying anything about it,
// for the caller that is about to describe the result itself.
func saveSanitizePlan(cfg *workingon.Config, plan []workingon.Adjustment) error {
	for _, adjustment := range plan {
		if _, err := saveAdjustment(cfg, adjustment); err != nil {
			return err
		}
	}

	return nil
}

// saveAdjustment writes one entry back, naming it in any failure - a plan that
// stopped halfway through is worth being specific about.
func saveAdjustment(cfg *workingon.Config, adjustment workingon.Adjustment) (*toggl.TimeEntry, error) {
	saved, err := workingon.Sanitize(cfg, adjustment)
	if err != nil {
		return nil, fmt.Errorf("unable to save %q: %w",
			describedAs(adjustment.Entry.Description), err)
	}

	return saved, nil
}
