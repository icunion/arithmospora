package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/coreos/go-systemd/daemon"

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

		// Perform periodic tasks: Periodically log number of connected clients and updates count
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

	// Determine server address
	address := ""
	if as.Config.Https.Address != "" {
		address = as.Config.Https.Address
	} else if as.Config.Http.Address != "" {
		address = as.Config.Http.Address
	} else {
		log.Fatal("Missing adress in configuration http or https section")
	}

	// Open socket
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatal("Cannot open listener: ", err)
	}

	// Notify systemd ready
	daemon.SdNotify(false, daemon.SdNotifyReady)

	// Setup systemd watchdog to periodically send keepalives
	go func() {
		interval, err := daemon.SdWatchdogEnabled(false)
		if err != nil || interval == 0 {
			return
		}
		// TODO: Add cleverness to actually monitor program is
		// working as expected
		for {
			daemon.SdNotify(false, daemon.SdNotifyWatchdog)
			time.Sleep(interval / 2)
		}
	}()

	// Launch server
	https := as.Config.Https
	if https.Cert != "" && https.Key != "" {
		log.Fatal(http.ServeTLS(listener, nil, https.Cert, https.Key))
	} else {
		log.Fatal(http.Serve(listener, nil))
	}
}
