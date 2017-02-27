package main

import (
	"flag"
	"fmt"
	"time"

	as "github.com/icunion/arithmospora"
)

var configFile = flag.String("c", "", "/path/to/configfile")
var sourceToWatch = flag.String("source", "", "Name of source to watch. Defaults to first source in config file")
var statToWatch = flag.String("stat", "all", "Name of stat to watch. Defaults to 'all' to watch all stats in the source")

func main() {
	// Load config
	flag.Parse()
	if err := as.ParseConfig(*configFile); err != nil {
		panic(err)
	}

	// Setup redis config
	as.SetRedisConfig(as.Config.Redis)

	// Set up sources
	sources := as.MakeSourcesFromConfig(as.Config)
	if *sourceToWatch == "" {
		*sourceToWatch = sources[0].Name
	}

	for _, source := range sources {
		if source.Name == *sourceToWatch {
			for _, stats := range source.Stats {
				for _, stat := range stats {
					if *statToWatch == "all" || *statToWatch == stat.Name {
						if err := printOnUpdate(stat, as.Config.Debounce); err != nil {
							fmt.Println(err)
							return
						}
					}
				}
			}
		}
	}

	for {
	}
}

func printOnUpdate(stat *as.Stat, debounce as.DebounceConfig) error {
	errors := make(chan error)
	if err := stat.ListenForUpdates(time.Duration(debounce.MinTimeMs)*time.Millisecond, time.Duration(debounce.MaxTimeMs)*time.Millisecond, errors); err != nil {
		return err
	}
	go func() {
		updated := make(chan bool)
		stat.RegisterListener(updated)
		for {
			select {
			case <-updated:
				fmt.Println(stat.String())
			case err := <-errors:
				fmt.Println(err)
			}
		}
	}()
	return nil
}
