package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fefeme/workingon/skills"
)

func TestSkillPath(t *testing.T) {
	t.Run("under the claude config directory where one is named", func(t *testing.T) {
		t.Setenv("CLAUDE_CONFIG_DIR", "/somewhere/.claude")

		got, err := skillPath("")
		if err != nil {
			t.Fatalf("skillPath: %v", err)
		}

		want := filepath.Join("/somewhere/.claude", "skills", "wo", "SKILL.md")
		if got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
	})

	t.Run("under the home directory otherwise", func(t *testing.T) {
		t.Setenv("CLAUDE_CONFIG_DIR", "")

		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("no home directory: %v", err)
		}

		got, err := skillPath("")
		if err != nil {
			t.Fatalf("skillPath: %v", err)
		}

		want := filepath.Join(home, ".claude", "skills", "wo", "SKILL.md")
		if got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
	})

	// A named directory is the skills directory itself, so the skill still gets
	// a directory of its own inside it - that is how it is addressed.
	t.Run("inside a directory named outright", func(t *testing.T) {
		t.Setenv("CLAUDE_CONFIG_DIR", "/ignored")

		got, err := skillPath("/elsewhere/skills")
		if err != nil {
			t.Fatalf("skillPath: %v", err)
		}

		want := filepath.Join("/elsewhere/skills", "wo", "SKILL.md")
		if got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
	})
}

func TestInstallSkillWritesTheWholeSkill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skills", "wo", "SKILL.md")

	if err := installSkill(path, false); err != nil {
		t.Fatalf("installSkill: %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	if string(written) != skills.Claude {
		t.Error("what was written is not the skill that was embedded")
	}
}

// A skill someone has edited is not something to discard without being asked.
func TestInstallSkillWillNotReplaceOneWithoutBeingTold(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skills", "wo", "SKILL.md")

	if err := installSkill(path, false); err != nil {
		t.Fatalf("the first install: %v", err)
	}
	if err := os.WriteFile(path, []byte("edited by hand"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := installSkill(path, false)
	if err == nil {
		t.Fatal("it replaced the skill without being told to")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error = %q, want it to say how to replace it anyway", err)
	}

	written, _ := os.ReadFile(path)
	if string(written) != "edited by hand" {
		t.Error("the refusal did not leave the file alone")
	}

	if err := installSkill(path, true); err != nil {
		t.Fatalf("installSkill --force: %v", err)
	}
	if written, _ := os.ReadFile(path); string(written) != skills.Claude {
		t.Error("--force did not replace the skill")
	}
}

// Installing over a checkout that was symlinked into place must not write
// through the link and edit the checkout.
func TestInstallSkillRefusesASymlinkedSkill(t *testing.T) {
	dir := t.TempDir()

	checkout := filepath.Join(dir, "checkout")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(checkout, "SKILL.md")
	if err := os.WriteFile(source, []byte("the checkout's own copy"), 0o644); err != nil {
		t.Fatal(err)
	}

	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(checkout, filepath.Join(skillsDir, "wo")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}

	path, err := skillPath(skillsDir)
	if err != nil {
		t.Fatalf("skillPath: %v", err)
	}

	if err := installSkill(path, false); err == nil {
		t.Fatal("it wrote through the symlink, want a refusal")
	}

	if written, _ := os.ReadFile(source); string(written) != "the checkout's own copy" {
		t.Error("the checkout was overwritten")
	}
}

// The embed is easy to break and silent when it does - an empty skill would
// install perfectly and teach nothing.
func TestTheEmbeddedSkillIsTheSkill(t *testing.T) {
	if !strings.HasPrefix(skills.Claude, "---\n") {
		t.Fatal("the skill does not begin with frontmatter")
	}
	if !strings.Contains(skills.Claude, "\nname: "+skills.ClaudeName+"\n") {
		t.Errorf("the frontmatter does not name it %q", skills.ClaudeName)
	}
	if !strings.Contains(skills.Claude, "--json") {
		t.Error("the skill does not mention --json, which is the whole point of it")
	}
}
