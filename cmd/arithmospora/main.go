package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	as "github.com/icunion/arithmospora"
)

var configFile = flag.String("c", "", "/path/to/configfile")

func main() {
	// Load config
	log.Printf("Loading config %s", *configFile)
	flag.Parse()
	if err := as.ParseConfig(*configFile); err != nil {
		log.Fatal("ParseConfig: ", err)
	}

	// Set config for redis and websocket
	log.Print("Setting config for Redis and WebSocket")
	as.SetRedisConfig(as.Config.Redis)
	as.SetWebSocketConfig(as.Config.Websocket)

	// Set up sources
	log.Print("Setting up sources")
	sources := as.MakeSourcesFromConfig(as.Config)

	// Set up error channel
	errors := make(chan error)
	go func() {
		for {
			err := <-errors
			log.Println(err)
		}
	}()

	// Set up websocket handlers per source
	for _, s := range sources {
		source := s
		log.Printf("Publishing source '%s'", source.Name)
		// Create Hub and run
		sourceHub := as.NewHub(source)
		go sourceHub.Run()
		http.HandleFunc("/"+source.Name, func(w http.ResponseWriter, r *http.Request) {
			as.ServeWs(sourceHub, w, r, errors)
		})

		// Publish source
		if err := source.Publish(sourceHub, errors); err != nil {
			log.Fatal("source.Publish: ", err)
		}

		// Perform period tasks Periodically log number of connected clients and updates count
		tickerLog := time.NewTicker(10 * time.Second)
		tickerRefresh := time.NewTicker(120 * time.Second)
		go func() {
			for {
				select {
				case <-tickerLog.C:
					log.Printf("Source '%s': %v clients; %v updates; %v milestones", source.Name, sourceHub.ClientCount(), source.PopUpdatesCounter(), source.PopMilestonesCounter())
				case <-tickerRefresh.C:
					if source.IsLive {
						log.Printf("Source '%s': periodic refresh", source.Name)
						source.RefreshAll(errors)
					}
				}
			}
		}()
	}

	// Launch server
	if httpsConf := as.Config.Https; httpsConf.Address != "" {
		err := http.ListenAndServeTLS(httpsConf.Address, httpsConf.Cert, httpsConf.Key, nil)
		if err != nil {
			log.Fatal("ListenAndServeTLS: ", err)
		}
	} else if httpConf := as.Config.Http; httpConf.Address != "" {
		err := http.ListenAndServe(httpConf.Address, nil)
		if err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
	} else {
		log.Fatal("Missing or bad http or htps section in configuration")
	}
}
