package arithmospora

import (
	"encoding/json"
	"sync"
	"time"
)

type Source struct {
	Name           string
	IsLive         bool
	Available      map[string][]string
	Stats          map[string]map[string]*Stat
	updatesCountMu sync.Mutex
	updatesCount   int
}

func (s *Source) Publish(hub *Hub, errors chan<- error) error {
	debounce := Config.Debounce

	for sg, stats := range s.Stats {
		statGroup := sg
		for sk, st := range stats {
			statKey := sk
			stat := st

			if s.IsLive {
				// Source is live: listen for updates
				if err := stat.ListenForUpdates(time.Duration(debounce.MinTimeMs)*time.Millisecond, time.Duration(debounce.MaxTimeMs)*time.Millisecond, errors); err != nil {
					return err
				}
			} else {
				// Source is not live: load data, but don't listen for updates
				if err := stat.Reload(); err != nil {
					return err
				}
			}

			go func() {
				updated := make(chan bool)
				stat.RegisterListener(updated)
				for {
					_ = <-updated
					s.IncrementUpdatesCounter()
					message, err := json.Marshal(Message{Event: "stats:" + statGroup + ":" + statKey, Payload: stat})
					if err != nil {
						errors <- err
						continue
					}
					hub.Broadcast <- message
				}
			}()
		}
	}

	return nil
}

func (s *Source) RefreshAll(errors chan<- error) {
	for _, stats := range s.Stats {
		for _, stat := range stats {
			if err := stat.Refresh(); err != nil {
				errors <- err
				continue
			}
			stat.NotifyListeners()
		}
	}
}

func (s *Source) SendInitialDataTo(client *Client) error {
	// Provide available stats message for this source
	available, err := json.Marshal(Message{Event: "available", Payload: s.Available})
	if err != nil {
		return err
	}
	client.send <- available

	// Send initial data after a short delay to allow the client time to
	// process available stats and set up listeners
	go func() {
		_ = <-time.After(100 * time.Millisecond)
		for statGroup, stats := range s.Stats {
			for statKey, stat := range stats {
				message, err := json.Marshal(Message{Event: "stats:" + statGroup + ":" + statKey, Payload: stat})
				if err != nil {
					continue
				}
				client.send <- message
			}
		}
	}()

	return nil
}

func (s *Source) IncrementUpdatesCounter() {
	s.updatesCountMu.Lock()
	s.updatesCount++
	s.updatesCountMu.Unlock()
}

func (s *Source) PopUpdatesCounter() (poppedCount int) {
	s.updatesCountMu.Lock()
	poppedCount = s.updatesCount
	s.updatesCount = 0
	s.updatesCountMu.Unlock()
	return poppedCount
}
