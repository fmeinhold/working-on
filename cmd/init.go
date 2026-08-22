package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/workingon"
	"github.com/spf13/cobra"
)

// initAnswers is everything `wo init` asks for, and everything the generated
// config needs.
type initAnswers struct {
	ApiToken       string
	WorkspaceId    int
	WorkspaceName  string
	Location       string
	DayFirst       bool
	DateLayout     string
	DateTimeLayout string
	WeekStarts     string
	PidRequired    bool
	TaskRequired   bool
	DefaultPid     int
}

// localAnswers is what `wo init` asks for once a global config exists: which
// project and task work done in this directory belongs to.
type localAnswers struct {
	ProjectId   int
	ProjectName string
	TaskId      int
	TaskName    string
}

// initSession is the io and api the command talks to, injected so the flow can
// be driven in a test.
type initSession struct {
	in     io.Reader
	out    io.Writer
	cfg    *workingon.Config
	token  string
	force  bool
	global bool
	local  bool
	path   string
	client func(apiToken string) *toggl.Toggl
}

func NewInitCommand(cfg *workingon.Config) *cobra.Command {
	session := initSession{
		in:     os.Stdin,
		out:    os.Stdout,
		cfg:    cfg,
		client: toggl.NewToggl,
	}

	command := &cobra.Command{
		Use:   "init",
		Short: "Create a config file",
		Long: `Create a config file.

With no config yet, this sets up the global one: it asks for your toggl api
token, checks it, and lets you pick a workspace and a default project from what
the account actually has. The result is written to
~/.config/working_on/config.yaml.

With a global config already in place, it sets up this repository instead,
asking which project and task work done here belongs to and writing a
` + workingon.LocalConfigName + ` overlay beside your checkout.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(session)
		},
	}

	command.Flags().StringVar(&session.token, "token", "",
		"Toggl api token, to avoid typing it where it can be seen")
	command.Flags().BoolVarP(&session.force, "force", "f", false,
		"Overwrite an existing config file")
	command.Flags().BoolVar(&session.global, "global", false,
		"Set up the global config, even if there already is one")
	command.Flags().BoolVar(&session.local, "local", false,
		"Set up this repository, writing a "+workingon.LocalConfigName+" overlay")
	command.Flags().StringVar(&session.path, "path", "",
		"Where to write the config (defaults to ~/.config/working_on/config.yaml, "+
			"or "+workingon.LocalConfigName+" for a repository)")

	return command
}

func runInit(session initSession) error {
	if session.global && session.local {
		return fmt.Errorf("--global and --local ask for different files - pick one")
	}

	if session.wantsLocal() {
		return runLocalInit(session)
	}

	return runGlobalInit(session)
}

// Once the global config exists, what is left to set up is the directory you
// are standing in.
func (s initSession) wantsLocal() bool {
	switch {
	case s.local:
		return true
	case s.global:
		return false
	default:
		return s.configured()
	}
}

// A config that names no token cannot reach toggl, so it is no better than
// none for the question of which flow to run.
func (s initSession) configured() bool {
	return s.cfg != nil && s.cfg.Settings.ToggleApiToken != ""
}

func runGlobalInit(session initSession) error {
	path := session.path
	if path == "" {
		resolved, err := defaultConfigPath()
		if err != nil {
			return err
		}
		path = resolved
	}

	if _, err := os.Stat(path); err == nil && !session.force {
		return fmt.Errorf("%s already exists - pass --force to replace it", path)
	}

	prompt := &prompter{reader: bufio.NewReader(session.in), out: session.out}

	answers, err := askInitQuestions(prompt, session)
	if err != nil {
		return err
	}

	rendered, err := renderConfig(answers)
	if err != nil {
		return err
	}

	if err := writeConfig(path, rendered); err != nil {
		return err
	}

	fmt.Fprintf(session.out, "\nWrote %s\n", path)
	fmt.Fprintf(session.out, "Tracking into %q.\n", answers.WorkspaceName)
	fmt.Fprintln(session.out, "\nTry `wo tasks` to see what you can book against, "+
		"then `wo start something` to begin.")

	return nil
}

// runLocalInit writes the per repository overlay: the project and task that
// entries created here default to.
func runLocalInit(session initSession) error {
	if !session.configured() {
		return fmt.Errorf("there is no global config to build on - run `wo init --global` first")
	}

	wid := session.cfg.Settings.ToggleWid
	if wid == 0 {
		return fmt.Errorf("the global config names no workspace (toggl_wid) to pick a project from")
	}

	path := session.path
	if path == "" {
		resolved, err := defaultLocalConfigPath()
		if err != nil {
			return err
		}
		path = resolved
	}

	if _, err := os.Stat(path); err == nil && !session.force {
		return fmt.Errorf("%s already exists - pass --force to replace it", path)
	}

	prompt := &prompter{reader: bufio.NewReader(session.in), out: session.out}
	client := session.client(session.cfg.Settings.ToggleApiToken)

	answers, err := askLocalQuestions(prompt, client, wid)
	if err != nil {
		return err
	}

	rendered, err := renderLocalConfig(answers)
	if err != nil {
		return err
	}

	if err := writeLocalConfig(path, rendered); err != nil {
		return err
	}

	fmt.Fprintf(session.out, "\nWrote %s\n", path)
	fmt.Fprint(session.out, localSummary(answers))

	return nil
}

func askLocalQuestions(prompt *prompter, client *toggl.Toggl, wid int) (localAnswers, error) {
	var answers localAnswers

	projects, err := activeProjects(client, wid)
	if err != nil {
		return answers, err
	}
	if len(projects) == 0 {
		return answers, fmt.Errorf("workspace %d has no active projects to choose from", wid)
	}

	project := pickProject(prompt, projects, "\nWhich project does work here belong to")
	if project == nil {
		return answers, fmt.Errorf("a project is what this file is for - nothing to write")
	}
	answers.ProjectId = project.Id
	answers.ProjectName = project.Name

	task, err := pickTask(prompt, client, wid, *project)
	if err != nil {
		return answers, err
	}
	if task != nil {
		answers.TaskId = task.Id
		answers.TaskName = task.Name
	}

	return answers, nil
}

func localSummary(answers localAnswers) string {
	if answers.TaskId == 0 {
		return fmt.Sprintf("Work here goes to %q, with no task.\n"+
			"\nTry `wo start something` to begin.\n", answers.ProjectName)
	}

	return fmt.Sprintf("Work here goes to %q, as %q.\n"+
		"\nTry `wo start something` to begin.\n", answers.ProjectName, answers.TaskName)
}

func askInitQuestions(prompt *prompter, session initSession) (initAnswers, error) {
	var answers initAnswers

	token, err := resolveApiToken(prompt, session)
	if err != nil {
		return answers, err
	}
	answers.ApiToken = token

	// Check the token before asking anything else - a wrong one should not be
	// discovered after five more questions.
	client := session.client(token)

	workspaces, err := client.WorkspaceClient.GetWorkspaces()
	if err != nil {
		return answers, fmt.Errorf("unable to reach toggl with that token: %w", err)
	}
	if workspaces.Count == 0 {
		return answers, fmt.Errorf("that token has no workspaces")
	}

	workspace, err := chooseWorkspace(prompt, workspaces.Workspaces)
	if err != nil {
		return answers, err
	}
	answers.WorkspaceId = workspace.Id
	answers.WorkspaceName = workspace.Name

	answers.Location = prompt.line("Timezone", localTimezone())

	answers.DateLayout = prompt.line("Date format", localeDateLayout())
	answers.DayFirst = dayFirstLayout(answers.DateLayout)
	answers.DateTimeLayout = prompt.line("Date and time format",
		answers.DateLayout+" "+localeTimeLayout())
	answers.WeekStarts = weekdayNamed(prompt.line("Week starts on", localeWeekStart()))

	answers.PidRequired = prompt.yesNo("Require a project on every entry", true)
	answers.TaskRequired = prompt.yesNo("Require a task on every entry", false)

	defaultPid, err := chooseDefaultProject(prompt, client, workspace.Id)
	if err != nil {
		return answers, err
	}
	answers.DefaultPid = defaultPid

	return answers, nil
}

func resolveApiToken(prompt *prompter, session initSession) (string, error) {
	if session.token != "" {
		return session.token, nil
	}

	fmt.Fprintln(prompt.out, "Your api token is at https://track.toggl.com/profile")
	fmt.Fprintln(prompt.out, "(it will be visible as you type - use --token to avoid that)")

	token := prompt.line("Api token", "")
	if token == "" {
		return "", fmt.Errorf("an api token is required")
	}

	return token, nil
}

func chooseWorkspace(prompt *prompter, workspaces []toggl.Workspace) (toggl.Workspace, error) {
	if len(workspaces) == 1 {
		fmt.Fprintf(prompt.out, "\nUsing workspace %q.\n", workspaces[0].Name)
		return workspaces[0], nil
	}

	fmt.Fprintln(prompt.out, "\nWorkspaces:")
	for i, workspace := range workspaces {
		fmt.Fprintf(prompt.out, "  %d) %s\n", i+1, workspace.Name)
	}

	for {
		answer := prompt.line("Which one", "1")

		choice, err := strconv.Atoi(answer)
		if err == nil && choice >= 1 && choice <= len(workspaces) {
			return workspaces[choice-1], nil
		}

		fmt.Fprintf(prompt.out, "Pick a number between 1 and %d.\n", len(workspaces))
	}
}

// chooseDefaultProject offers the workspace's projects as the fallback for
// entries that name no project of their own.
func chooseDefaultProject(prompt *prompter, client *toggl.Toggl, wid int) (int, error) {
	projects, err := activeProjects(client, wid)
	if err != nil {
		// Not worth failing setup over; the config is usable without one.
		fmt.Fprintf(prompt.out, "\nCould not list projects (%v), skipping the default.\n", err)
		return 0, nil
	}
	if len(projects) == 0 {
		return 0, nil
	}

	if !prompt.yesNo("\nPick a default project for entries that name none", false) {
		return 0, nil
	}

	project := pickProject(prompt, projects, "Which one (blank for none)")
	if project == nil {
		return 0, nil
	}

	return project.Id, nil
}

// activeProjects are the projects worth offering. Archived ones are a poor
// default for entries yet to be created, and there are usually far more of them
// than live ones.
func activeProjects(client *toggl.Toggl, wid int) ([]toggl.Project, error) {
	projects, err := client.WorkspaceClient.ListProjectsWhere(wid,
		toggl.ProjectQuery{ActiveOnly: true})
	if err != nil {
		return nil, err
	}
	return projects.Projects, nil
}

func pickProject(prompt *prompter, projects []toggl.Project, question string) *toggl.Project {
	names := make([]string, len(projects))
	for i, project := range projects {
		names[i] = project.Name
	}

	index := pickOne(prompt, question, names)
	if index < 0 {
		return nil
	}

	return &projects[index]
}

// pickTask offers the tasks of the chosen project, for entries that name none
// of their own.
func pickTask(prompt *prompter, client *toggl.Toggl, wid int, project toggl.Project) (*toggl.Task, error) {
	tasks, err := client.TaskClient.List(wid)
	if err != nil {
		// A project alone is a useful overlay, so this is not fatal.
		fmt.Fprintf(prompt.out, "\nCould not list tasks (%v), skipping the default.\n", err)
		return nil, nil
	}

	var active []toggl.Task
	for _, task := range tasks.Tasks {
		if task.ProjectId == project.Id && task.Active && !task.IsDeleted() {
			active = append(active, task)
		}
	}

	if len(active) == 0 {
		fmt.Fprintf(prompt.out, "\n%s has no active tasks.\n", project.Name)
		return nil, nil
	}

	fmt.Fprintf(prompt.out, "\nTasks in %s:\n", project.Name)

	names := make([]string, len(active))
	for i, task := range active {
		names[i] = task.Name
	}

	index := pickOne(prompt, "Which task should new entries default to (blank for none)", names)
	if index < 0 {
		return nil, nil
	}

	return &active[index], nil
}

// choicesShown caps a listing at what someone can still read in a terminal.
// Everything past it is still reachable, by typing part of a name.
const choicesShown = 20

// showAll is the answer that undoes a filter, since a blank line already means
// "none of these".
const showAll = "*"

// pickOne lists the options and reads a choice, returning the index of the one
// picked or -1 for a blank answer.
//
// An answer that is not a number narrows the listing to the options containing
// it, which is how anything past choicesShown is chosen. Narrowing always
// starts from the full list, so a second filter replaces the first rather than
// digging further into it.
func pickOne(prompt *prompter, question string, names []string) int {
	return pickOneMatching(prompt, question, names, matching)
}

// pickOneMatching is pickOne with the narrowing left to the caller, for a
// listing whose own idea of a match is not "contains this text".
func pickOneMatching(prompt *prompter, question string, names []string,
	match func(names []string, filter string) []int) int {

	matches := make([]int, len(names))
	for i := range names {
		matches[i] = i
	}

	shown := listChoices(prompt, names, matches, "")

	for {
		answer := prompt.line(question, "")
		if answer == "" {
			return -1
		}

		if choice, err := strconv.Atoi(answer); err == nil {
			if choice >= 1 && choice <= shown {
				return matches[choice-1]
			}

			fmt.Fprintf(prompt.out, "Pick a number between 1 and %d, "+
				"type part of a name to narrow the list, or leave it blank.\n", shown)
			continue
		}

		filter := answer
		if filter == showAll {
			filter = ""
		}

		narrowed := match(names, filter)
		if len(narrowed) == 0 {
			fmt.Fprintf(prompt.out, "Nothing matches %q - try something shorter, "+
				"or %s for the whole list.\n", answer, showAll)
			continue
		}

		matches = narrowed
		shown = listChoices(prompt, names, matches, filter)
	}
}

func matching(names []string, filter string) []int {
	wanted := strings.ToLower(filter)

	var found []int
	for i, name := range names {
		if strings.Contains(strings.ToLower(name), wanted) {
			found = append(found, i)
		}
	}

	return found
}

// listChoices prints a numbered listing of matches, capped at choicesShown, and
// returns how many rows it printed - the range a numeric answer may name.
func listChoices(prompt *prompter, names []string, matches []int, filter string) int {
	shown := len(matches)
	if shown > choicesShown {
		shown = choicesShown
	}

	if filter != "" {
		fmt.Fprintf(prompt.out, "\n%d matching %q:\n", len(matches), filter)
	}

	for i := 0; i < shown; i++ {
		fmt.Fprintf(prompt.out, "  %d) %s\n", i+1, names[matches[i]])
	}

	switch hidden := len(matches) - shown; {
	case hidden > 0:
		fmt.Fprintf(prompt.out, "  ... %d more - type part of a name to narrow the list\n", hidden)
	case filter != "":
		fmt.Fprintf(prompt.out, "  (%s for the whole list)\n", showAll)
	}

	return shown
}

type prompter struct {
	reader *bufio.Reader
	out    io.Writer
}

func (p *prompter) line(question string, fallback string) string {
	if fallback != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", question, fallback)
	} else {
		fmt.Fprintf(p.out, "%s: ", question)
	}

	answer, err := p.reader.ReadString('\n')
	answer = strings.TrimSpace(answer)

	if answer == "" {
		if err != nil && fallback == "" {
			// Nothing left to read and nothing to fall back on.
			return ""
		}
		return fallback
	}

	return answer
}

func (p *prompter) yesNo(question string, fallback bool) bool {
	shown := "y/N"
	if fallback {
		shown = "Y/n"
	}

	fmt.Fprintf(p.out, "%s [%s]: ", question, shown)

	answer, _ := p.reader.ReadString('\n')

	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return fallback
	}
}

// Testing the first character is not enough: "2006-01-02" also starts with a 2.
func dayFirstLayout(layout string) bool {
	fields := isSeparator.Split(layout, -1)
	if len(fields) == 0 {
		return false
	}
	return dateFieldKind(fields[0]) == "day"
}

// localTimezone is this machine's IANA zone name, for the default answer.
//
// time.Local usually reports "Local" rather than a zone name, so fall back to
// whatever /etc/localtime points at - a path ending in <Area>/<Zone> under a
// zoneinfo directory.
func localTimezone() string {
	if tz, set := os.LookupEnv("TZ"); set && tz != "" {
		return tz
	}

	// "Local" is the usual answer, and "UTC" is what Go reports when TZ is set
	// but empty - neither says anything about where the machine actually is.
	if name := time.Local.String(); name != "" && name != "Local" && name != "UTC" {
		return name
	}

	if target, err := filepath.EvalSymlinks(localTimeFile); err == nil {
		if zone := zoneFromPath(target); zone != "" {
			return zone
		}
	}

	return "UTC"
}

// localTimeFile is a variable so a test can point it somewhere predictable.
var localTimeFile = "/etc/localtime"

func zoneFromPath(path string) string {
	const marker = "zoneinfo/"

	index := strings.LastIndex(path, marker)
	if index < 0 {
		return ""
	}

	return strings.Trim(path[index+len(marker):], "/")
}

func defaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "working_on", "config.yaml"), nil
}

// defaultLocalConfigPath is the repository root when we are standing in a
// checkout, and the working directory otherwise. The overlay applies from where
// it sits downwards, so the root is where it covers the whole project rather
// than whichever subdirectory `wo init` happened to be run in.
func defaultLocalConfigPath() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	if root := repositoryRoot(dir); root != "" {
		dir = root
	}

	return filepath.Join(dir, workingon.LocalConfigName), nil
}

func repositoryRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// The file and its directory are both private: it holds an api token.
func writeConfig(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

// The overlay holds no credentials - they are ignored from this file - so it is
// readable like the rest of the checkout it is likely to be committed to.
func writeLocalConfig(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func renderConfig(answers initAnswers) (string, error) {
	return render(configTemplate, answers)
}

func renderLocalConfig(answers localAnswers) (string, error) {
	return render(localConfigTemplate, answers)
}

func render(text string, answers interface{}) (string, error) {
	tpl, err := template.New("config").Parse(text)
	if err != nil {
		return "", err
	}

	var rendered strings.Builder
	if err := tpl.Execute(&rendered, answers); err != nil {
		return "", err
	}

	return rendered.String(), nil
}

// configTemplate mirrors config.example.yaml, keeping the explanation next to
// the setting it explains.
const configTemplate = `# Working On - written by ` + "`wo init`" + `
#
# Per repository overrides go in a .workingon.yaml beside your checkout; see
# config.example.yaml in the repository for what those can carry.

settings:
  day_first: {{.DayFirst}}

  # An IANA timezone name. Times you type are read in this zone, stored in
  # UTC, and displayed back in this zone.
  location: {{.Location}}

  # Go reference layouts - https://pkg.go.dev/time#pkg-constants
  # The layout also decides how partial dates are read.
  date_layout: "{{.DateLayout}}"
  date_time_layout: "{{.DateTimeLayout}}"

  # The day a week is read from, for wo show --week.
  week_starts: {{.WeekStarts}}

  # From https://track.toggl.com/profile
  toggl_api_token: {{.ApiToken}}

  # {{.WorkspaceName}}
  toggl_wid: {{.WorkspaceId}}

  # A time entry's project is resolved in this order:
  #   1. the project the task belongs to, for an entry that ended up with one
  #   2. the --project flag, by id or by project name
  #   3. toggl_default_pid
  # The task leads because it belongs to exactly one project, and toggl refuses
  # an entry filing it under another.
  #
  # If none of those produce one, toggl_pid_required decides whether that is
  # an error or an entry with no project.
  #
  # Its task, in this order:
  #   1. the --task flag, by id or by name
  #   2. a task named as the summary  (wo add "Some Task" 2h)
  #   3. a task referenced by id      (wo add 241929955 2h)
  #   4. toggl_default_task, for an entry that landed in toggl_default_pid
  toggl_pid_required: {{.PidRequired}}
  toggl_default_pid: {{.DefaultPid}}

  # Some workspaces want a task on every entry. Where that holds, ` + "`wo`" + ` asks
  # which one rather than letting toggl refuse the entry, and --pick-task asks
  # even where it does not.
  toggl_task_required: {{.TaskRequired}}

  # The task new entries fall back to, used only for entries landing in
  # toggl_default_pid. ` + "`wo init`" + ` inside a repository sets this per checkout.
  toggl_default_task: 0

  # What to call an entry that has no description, for a workspace that requires
  # one. Left empty, ` + "`wo`" + ` asks; a run with nowhere to ask says "Untitled".
  toggl_default_description: ""


# Time entry templates, addressed by their alias: ` + "`wo add ds`" + ` books the
# daily standup. Descriptions are Go templates, filled from --templateArgs;
# ` + "`wo`" + ` asks for a placeholder no argument answered for.
# toggl_pid and toggl_task are optional, and pin where the entry lands.
#
#   - alias: "ds"
#     description: "Daily Standup"
#     toggl_pid: 12345678
#     toggl_task: 87654321
#     start: "17:30"
#     stop: "17:45"
templates: []


# How ` + "`wo sanitize`" + ` tidies a day. no_work are hours nothing is stretched
# into, as "12:00-13:00"; snap is the grid times are rounded to, and short the
# length under which an entry takes the gaps around it. Write "0" for none.
#
# day_ends is when work stops, as "18:00". An entry that outlived it is cut back
# there, which is what a timer left running overnight comes to. Empty caps
# nothing.
sanitize:
  snap: "5m"
  short: "15m"
  no_work: []
  day_ends: ""


# Task sources other than toggl. Toggl is built in and takes its credentials
# from settings above.
sources: {}
`

// localConfigTemplate is the repository overlay: the little of the global
// config that differs for work done here.
const localConfigTemplate = `# Working On - settings for this repository, written by ` + "`wo init`" + `
#
# Merged over ~/.config/working_on/config.yaml for work done here, so it only
# needs the keys it changes. It is looked for from the working directory
# upwards, stopping at the repository root. Credentials are ignored in this
# file: a checked in overlay cannot change which account you authenticate as.

settings:
  # {{.ProjectName}}
  toggl_default_pid: {{.ProjectId}}
{{if .TaskId}}
  # {{.TaskName}} - what a new entry books against unless it names a task of
  # its own, or is given one with --task.
  toggl_default_task: {{.TaskId}}
{{end}}`

// weekdayNamed settles what was typed at the week start question into the name
// the setting is written with, so "Sun" and "sunday" both land as "sunday". An
// answer that is no day at all is written down as it was typed: `wo show
// --week` says what is wrong with it far better than a prompt refusing an
// answer during setup.
func weekdayNamed(answer string) string {
	if day, known := weekdays[strings.ToLower(strings.TrimSpace(answer))]; known {
		return strings.ToLower(day.String())
	}
	return strings.TrimSpace(answer)
}
