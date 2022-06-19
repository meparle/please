package core

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/thought-machine/please/src/cli"
)

// A Subrepo stores information about a registered subrepository, typically one
// that we have downloaded somehow to bring in third-party deps.
type Subrepo struct {
	// The name of the subrepo.
	Name string
	// The root directory to load it from.
	Root string
	// If this repo is output by a target, this is the target that creates it. Can be nil.
	Target *BuildTarget
	// The build state instance that tracks this subrepo (it's different from the host one if
	// this subrepo is for a different architecture)
	State *BuildState
	// Architecture for this subrepo.
	Arch cli.Arch
	// True if this subrepo was created for a different architecture
	IsCrossCompile bool
	// loadConfig is used to control when we load plugin configuration. We need access to the subrepo itself to do this
	// so it happens at build time.
	loadConfig            sync.Once
	AdditionalConfigFiles []string
}

// SubrepoForArch creates a new subrepo for the given architecture.
func SubrepoForArch(state *BuildState, arch cli.Arch) *Subrepo {
	return &Subrepo{
		Name:           arch.String(),
		State:          state.ForArch(arch),
		Arch:           arch,
		IsCrossCompile: true,
	}
}

// SubrepoArchName returns the subrepo name augmented for the given architecture
func SubrepoArchName(subrepo string, arch cli.Arch) string {
	return subrepo + "_" + arch.String()
}

// Dir returns the directory for a package of this name.
func (s *Subrepo) Dir(dir string) string {
	return path.Join(s.Root, dir)
}

func readConfigFilesInto(config, repoConfig *Configuration, files []string) error {
	for _, file := range files {
		err := readConfigFile(config, file, true)
		if err != nil {
			return err
		}

		err = readConfigFile(repoConfig, file, true)
		if err != nil {
			return err
		}
	}
	return nil
}

// LoadSubrepoConfig will load the .plzconfig from the subrepo. We can only do this once the subrepo is built hence why
// it's not done up front.
func (s *Subrepo) LoadSubrepoConfig() (err error) {
	s.loadConfig.Do(func() {
		s.State.RepoConfig = &Configuration{}

		err = readConfigFilesInto(s.State.Config, s.State.RepoConfig, append(s.AdditionalConfigFiles, filepath.Join(s.Root, ".plzconfig")))
		if err != nil {
			return
		}
		if err = validateSubrepoNameAndPluginConfig(s.State.Config, s.State.RepoConfig, s); err != nil {
			return
		}
		go s.State.Parser.Init(s.State)
	})
	return
}

func validateSubrepoNameAndPluginConfig(config, repoConfig *Configuration, subrepo *Subrepo) error {
	// Validate plugin ID is the same as the subrepo name
	if pluginID := repoConfig.PluginDefinition.Name; pluginID != "" {
		subrepoName := subrepo.Name
		if subrepo.Arch.String() != "" {
			subrepoName = strings.TrimSuffix(subrepo.Name, "_"+subrepo.Arch.String())
		}
		if !strings.EqualFold(pluginID, subrepoName) {
			return fmt.Errorf("Subrepo name %q should be the same as the plugin ID %q", subrepoName, pluginID)
		}
	}

	// Validate the plugin config keys set in the host repo
	definedKeys := map[string]bool{}
	for key, definition := range repoConfig.PluginConfig {
		configKey := getConfigKey(key, definition.ConfigKey)
		definedKeys[configKey] = true
	}
	if plugin := config.Plugin[subrepo.Name]; plugin != nil {
		for key := range plugin.ExtraValues {
			if _, ok := definedKeys[strings.ToLower(key)]; !ok {
				return fmt.Errorf("Unrecognised config key %q for plugin %q %v %v", key, subrepo.Name, plugin.ExtraValues, definedKeys)
			}
		}
	}
	return nil
}

func getConfigKey(aspKey, configKey string) string {
	if configKey == "" {
		configKey = strings.ReplaceAll(aspKey, "_", "")
	}
	return strings.ToLower(configKey)
}
