package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fefeme/workingon/skills"
	"github.com/spf13/cobra"
)

func NewSkillCommand() *cobra.Command {
	var (
		install bool
		force   bool
		dir     string
	)

	skillCommand := &cobra.Command{
		Use:   "skill",
		Short: "Print the Claude Code skill for driving wo",
		Long: `Print the Claude Code skill for driving wo.

It teaches an agent to start and stop timers, to read the ` + "`--json`" + ` output
rather than the prose, and to leave a checkout alone where there is no
` + "`.workingon.yaml`" + ` to say what its time belongs to.

Written to stdout, so it can be read or redirected. --install writes it to the
skills directory instead, creating what it needs:

  wo skill --install

That is ~/.claude/skills/wo/SKILL.md, or the same path under $CLAUDE_CONFIG_DIR
where that is set. --dir puts it somewhere else. An existing skill is left
alone unless --force says otherwise, so a run of this never quietly discards
one you have edited.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := skillPath(dir)
			if err != nil {
				return err
			}

			if install {
				if err := installSkill(path, force); err != nil {
					return err
				}
			}

			if jsonOutput {
				return emit(struct {
					Path      string `json:"path"`
					Installed bool   `json:"installed"`
					Skill     string `json:"skill"`
				}{path, install, skills.Claude})
			}

			if install {
				fmt.Printf("Wrote the skill to %s\n", path)
				return nil
			}

			fmt.Print(skills.Claude)

			return nil
		},
	}

	skillCommand.Flags().BoolVar(&install, "install", false,
		"Write the skill to the skills directory instead of to stdout")
	skillCommand.Flags().BoolVar(&force, "force", false,
		"Overwrite a skill that is already there")
	skillCommand.Flags().StringVar(&dir, "dir", "",
		"Skills directory to install into (default ~/.claude/skills)")

	return skillCommand
}

// CLAUDE_CONFIG_DIR is honoured because Claude Code honours it, and a skill
// written anywhere else would simply never be read.
func skillPath(dir string) (string, error) {
	if dir == "" {
		config := os.Getenv("CLAUDE_CONFIG_DIR")
		if config == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("unable to find your home directory: %w", err)
			}
			config = filepath.Join(home, ".claude")
		}
		dir = filepath.Join(config, "skills")
	}

	return filepath.Join(dir, skills.ClaudeName, "SKILL.md"), nil
}

// Refusing to replace a skill that is already there covers the case of one
// installed by hand from a checkout, where the path is a symlink back into it:
// overwriting would edit the checkout, which is not what installing sounds
// like it does.
func installSkill(path string, force bool) error {
	if !force {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("%s already exists - use --force to replace it", path)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("unable to create %s: %w", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, []byte(skills.Claude), 0o644); err != nil {
		return fmt.Errorf("unable to write %s: %w", path, err)
	}

	return nil
}
