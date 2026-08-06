package workingon

import (
	"github.com/tcnksm/go-gitconfig"
)

// FindMappingByGitRepositoryUrl returns the mapping whose git remote matches
// the repository we are standing in, or nil when there is no match.
func FindMappingByGitRepositoryUrl(cfg *Config) *ProjectMapping {
	url, _ := gitconfig.OriginURL()
	if url == "" {
		return nil
	}

	for i := range cfg.Projects {
		if cfg.Projects[i].Git == url {
			return &cfg.Projects[i]
		}
	}

	return nil
}
