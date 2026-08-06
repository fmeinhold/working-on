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
	PidRequired    bool
	DefaultPid     int
}

// initSession is the io and api the command talks to, injected so the flow can
// be driven in a test.
type initSession struct {
	in     io.Reader
	out    io.Writer
	token  string
	force  bool
	path   string
	client func(apiToken string) *toggl.Toggl
}

func NewInitCommand() *cobra.Command {
	session := initSession{
		in:     os.Stdin,
		out:    os.Stdout,
		client: toggl.NewToggl,
	}

	command := &cobra.Command{
		Use:   "init",
		Short: "Create a config file",
		Long: `Create a config file.

Asks for your toggl api token, checks it, and lets you pick a workspace and a
default project from what the account actually has. Writes the result to
~/.config/working_on/config.yaml.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(session)
		},
	}

	command.Flags().StringVar(&session.token, "token", "",
		"Toggl api token, to avoid typing it where it can be seen")
	command.Flags().BoolVarP(&session.force, "force", "f", false,
		"Overwrite an existing config file")
	command.Flags().StringVar(&session.path, "path", "",
		"Where to write the config (defaults to ~/.config/working_on/config.yaml)")

	return command
}

func runInit(session initSession) error {
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

	answers.DateLayout = prompt.line("Date format", "2.1.2006")
	answers.DayFirst = dayFirstLayout(answers.DateLayout)
	answers.DateTimeLayout = prompt.line("Date and time format", answers.DateLayout+" 15:04")

	answers.PidRequired = prompt.yesNo("Require a project on every entry", true)

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
	projects, err := client.WorkspaceClient.ListProjects(wid)
	if err != nil {
		// Not worth failing setup over; the config is usable without one.
		fmt.Fprintf(prompt.out, "\nCould not list projects (%v), skipping the default.\n", err)
		return 0, nil
	}
	if projects.Count == 0 {
		return 0, nil
	}

	if !prompt.yesNo("\nPick a default project for entries that name none", false) {
		return 0, nil
	}

	active := make([]toggl.Project, 0, projects.Count)
	for _, project := range projects.Projects {
		if project.Active {
			active = append(active, project)
		}
	}
	if len(active) == 0 {
		active = projects.Projects
	}

	shown := active
	if len(shown) > 20 {
		shown = shown[:20]
		fmt.Fprintf(prompt.out, "(showing the first 20 of %d)\n", len(active))
	}

	for i, project := range shown {
		fmt.Fprintf(prompt.out, "  %d) %s\n", i+1, project.Name)
	}

	for {
		answer := prompt.line("Which one (blank for none)", "")
		if answer == "" {
			return 0, nil
		}

		choice, err := strconv.Atoi(answer)
		if err == nil && choice >= 1 && choice <= len(shown) {
			return shown[choice-1].Id, nil
		}

		fmt.Fprintf(prompt.out, "Pick a number between 1 and %d, or leave it blank.\n", len(shown))
	}
}

// prompter asks questions on out and reads answers from reader.
type prompter struct {
	reader *bufio.Reader
	out    io.Writer
}

// line asks a question, returning fallback when the answer is empty.
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

// dayFirstLayout reports whether a date layout leads with the day. Testing the
// first character is not enough: "2006-01-02" also starts with a 2.
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

// zoneFromPath pulls "Europe/Berlin" out of a zoneinfo path.
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

// writeConfig writes the config, creating its directory. Both are private:
// the file holds an api token.
func writeConfig(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

func renderConfig(answers initAnswers) (string, error) {
	tpl, err := template.New("config").Parse(configTemplate)
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

  # From https://track.toggl.com/profile
  toggl_api_token: {{.ApiToken}}

  # {{.WorkspaceName}}
  toggl_wid: {{.WorkspaceId}}

  # A time entry's project is resolved in this order:
  #   1. the --project flag
  #   2. the project the task belongs to
  #   3. a mapping below matching this repository's git remote
  #   4. toggl_default_pid
  # If none of those produce one, toggl_pid_required decides whether that is
  # an error or an entry with no project.
  #
  # Its task, in this order:
  #   1. the --task flag, by id or by name
  #   2. a task named as the summary  (wo add "Some Task" 2h)
  #   3. a task referenced by id      (wo add 241929955 2h)
  #   4. the toggl_task of the mapping this repository matches
  toggl_pid_required: {{.PidRequired}}
  toggl_default_pid: {{.DefaultPid}}


# Map a project name or a git repository to a toggl project, and optionally to
# a task within it. The git entry is matched against the repository's origin
# url, so work done in that checkout is filed automatically.
#
#   - name: "EXAMPLE"
#     toggl_pid: 12345678
#     toggl_task: 87654321
#     git: git@github.com:you/example.git
mappings: []


# Time entry templates, addressed by their alias: ` + "`wo add ds`" + ` books the
# daily standup. Descriptions are Go templates, filled from --templateArgs.
#
#   - alias: "ds"
#     description: "Daily Standup"
#     start: "17:30"
#     stop: "17:45"
templates: []


# Task sources other than toggl. Toggl is built in and takes its credentials
# from settings above.
sources: {}
`
