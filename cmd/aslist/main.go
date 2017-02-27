package main

import (
	"encoding/json"
	"flag"
	"fmt"

	as "github.com/icunion/arithmospora"
)

var configFile = flag.String("c", "", "/path/to/configfile")
var sourceToWatch = flag.String("source", "", "Name of source to list. Defaults to first source in config file")
var outputFormat = flag.String("f", "text", "Output format: text or json")
var prettyPrint = flag.Bool("pp", false, "Pretty print json (ignored if format is text)")

func main() {
	// Load config
	flag.Parse()
	if err := as.ParseConfig(*configFile); err != nil {
		panic(err)
	}

	// Setup redis config
	as.SetRedisConfig(as.Config.Redis)

	// Load sources
	sources := as.MakeSourcesFromConfig(as.Config)
	if *sourceToWatch == "" {
		*sourceToWatch = sources[0].Name
	}

	// Setup sourceOutput for JSON
	sourceOutput := make(map[string]map[string]*as.Stat)

	// Load stat data and print
	for _, source := range sources {
		if source.Name == *sourceToWatch {
			for statGroup, stats := range source.Stats {
				if *outputFormat == "text" {
					fmt.Printf("Stat group: %s\n", statGroup)
				}
				for _, stat := range stats {
					if err := stat.Reload(); err != nil {
						fmt.Println(err)
						return
					}

					if *outputFormat == "json" {
						if sourceOutput[statGroup+"Stats"] == nil {
							sourceOutput[statGroup+"Stats"] = make(map[string]*as.Stat)
						}
						sourceOutput[statGroup+"Stats"][stat.Name] = stat
					} else {
						fmt.Println(stat)
					}
				}
			}
			if *outputFormat == "json" {
				var (
					sourceJSON []byte
					err        error
				)
				if *prettyPrint {
					sourceJSON, err = json.MarshalIndent(sourceOutput, "", "    ")
				} else {
					sourceJSON, err = json.Marshal(sourceOutput)
				}
				if err != nil {
					fmt.Println(err)
					return
				}
				fmt.Printf("%s\n", sourceJSON)
			}
		}
	}
}
