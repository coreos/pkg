package yamlutil

import (
	"flag"
	"fmt"
	"strings"

	"gopkg.in/yaml.v1"
)

// SetFlagsFromYaml goes through all registered flags in the given flagset,
// and if they are not already set it attempts to set their values from
// the YAML config. It will use the key REPLACE(UPPERCASE(flagname), '-', '_')
func SetFlagsFromYaml(fs *flag.FlagSet, rawYaml []byte) (err error) {
	conf := make(map[string]string)
	if err = yaml.Unmarshal(rawYaml, conf); err != nil {
		return
	}
	alreadySet := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) {
		alreadySet[f.Name] = true
	})
	fs.VisitAll(func(f *flag.Flag) {
		if !alreadySet[f.Name] {
			tag := strings.ToUpper(f.Name)
			tag = strings.Replace(tag, "-", "_", -1)
			if tag != "" {
				val, ok := conf[tag]
				if !ok {
					return
				}
				if serr := fs.Set(f.Name, val); serr != nil {
					err = fmt.Errorf("invalid value %q for %s: %v", val, tag, serr)
				}
			}
		}
	})
	return
}
