package workingon

import (
	"bytes"
	"text/template"
	"text/template/parse"

	"github.com/fefeme/workingon/toggl"
	"github.com/fefeme/workingon/util"
)

// TemplateArgAsker answers for the placeholders in a template's description
// that --templateArgs left open. It is given them all at once, so it can say
// which template is asking before running through them, and may answer fewer
// than it was asked about.
type TemplateArgAsker func(alias string, names []string) (map[string]string, error)

// openArgs are the placeholders the description refers to that templateArgs has
// no answer for, in the order they first appear.
//
// An argument given as empty counts as answered: someone who wrote
// `-t caller=` meant the description to read that way.
func (t *TemplateConfig) openArgs(templateArgs map[string]string) []string {
	var open []string

	for _, name := range placeholders(t.Description) {
		if _, answered := templateArgs[name]; !answered {
			open = append(open, name)
		}
	}

	return open
}

// With nobody to ask - a script, a cron job - the arguments are handed back as
// they came and an open placeholder renders as "<no value>", which is what it
// has always done.
func (t *TemplateConfig) answerOpenArgs(templateArgs map[string]string,
	ask TemplateArgAsker) (map[string]string, error) {

	if ask == nil {
		return templateArgs, nil
	}

	open := t.openArgs(templateArgs)
	if len(open) == 0 {
		return templateArgs, nil
	}

	answers, err := ask(t.Alias, open)
	if err != nil {
		return nil, err
	}

	// The map came from the command line flags, and belongs to the caller - the
	// answers go into a copy of it so a question answered for this entry cannot
	// follow the flags anywhere else.
	filled := make(map[string]string, len(templateArgs)+len(answers))
	for name, value := range templateArgs {
		filled[name] = value
	}
	for _, name := range open {
		if answer, answered := answers[name]; answered && answer != "" {
			filled[name] = answer
		}
	}

	return filled, nil
}

// placeholders are the arguments a description asks for - the "caller" of
// "Call with {{.caller}}" - in the order they first appear.
//
// A description that does not parse asks for nothing: rendering it reports the
// fault a moment later, and there is nothing worth asking about until it is
// fixed.
func placeholders(description string) []string {
	tpl, err := template.New("t").Parse(description)
	if err != nil {
		return nil
	}

	var (
		names  []string
		seen   = map[string]bool{}
		walk   func(node parse.Node)
		branch func(node parse.BranchNode)
	)

	walk = func(node parse.Node) {
		switch n := node.(type) {
		case *parse.ListNode:
			if n == nil {
				return
			}
			for _, child := range n.Nodes {
				walk(child)
			}
		case *parse.ActionNode:
			walk(n.Pipe)
		case *parse.PipeNode:
			if n == nil {
				return
			}
			for _, command := range n.Cmds {
				walk(command)
			}
		case *parse.CommandNode:
			for _, arg := range n.Args {
				walk(arg)
			}
		case *parse.IfNode:
			branch(n.BranchNode)
		case *parse.RangeNode:
			branch(n.BranchNode)
		case *parse.WithNode:
			branch(n.BranchNode)
		case *parse.FieldNode:
			// {{.caller.name}} is answered by giving "caller", so it is the
			// first part of the path that is asked about.
			if len(n.Ident) > 0 && !seen[n.Ident[0]] {
				seen[n.Ident[0]] = true
				names = append(names, n.Ident[0])
			}
		}
	}

	branch = func(node parse.BranchNode) {
		walk(node.Pipe)
		walk(node.List)
		walk(node.ElseList)
	}

	walk(tpl.Tree.Root)

	return names
}

func (t *TemplateConfig) CreateTimeEntryFromTemplate(templateArgs map[string]string) (*toggl.TimeEntry, error) {

	tpl, err := template.New("t").Parse(t.Description)
	if err != nil {
		return nil, err
	}

	var description bytes.Buffer

	err = tpl.Execute(&description, templateArgs)
	if err != nil {
		return nil, err
	}

	// A template may pin the project and task its entries belong to. Both are
	// left for the caller to settle when it does not: an explicit --project
	// still wins over either, and the configured default still stands in.
	timeEntry := toggl.TimeEntry{
		Description: description.String(),
		ProjectId:   t.TogglPid,
		TaskId:      t.TogglTask,
		Billable:    false,
		CreatedWith: toggl.CreatedWith,
	}

	if t.Start != "" {
		start, err := util.ParseTimeUTCE(t.Start, Configuration.Settings.DateLayout,
			Configuration.Settings.DateTimeLayout, &Configuration.Settings.Location)
		if err != nil {
			return nil, err
		}
		timeEntry.Start = &start
	}
	if t.Stop != "" {
		stop, err := util.ParseTimeUTCE(t.Stop, Configuration.Settings.DateLayout,
			Configuration.Settings.DateTimeLayout, &Configuration.Settings.Location)
		if err != nil {
			return nil, err
		}
		timeEntry.Stop = &stop
	}

	// A stop on its own says nothing about how long the entry ran, so both ends
	// have to be there before a duration can be worked out.
	if timeEntry.Start != nil && timeEntry.Stop != nil && timeEntry.Duration == 0 {
		timeEntry.Duration = int64(timeEntry.Stop.Sub(*timeEntry.Start).Seconds())
	}

	return &timeEntry, nil

}
