package arithmospora

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type StatData interface {
	Refresh() error
	json.Marshaler
	fmt.Stringer
}

type StatDataLoader interface {
	Load(*Stat) (StatData, error)
	fmt.Stringer
}

type StatDataPointLoader interface {
	DataPointNames() ([]string, error)
	NewDataLoader(StatDataLoader, string) StatDataLoader
	NewDataPointLoader(string) StatDataPointLoader
}

type StatUpdateListener interface {
	Subscribe(chan<- bool)
}

type Stat struct {
	sync.Mutex
	Name            string
	Depth           int
	DataLoader      StatDataLoader
	DataPointLoader StatDataPointLoader
	UpdateListener  StatUpdateListener
	data            StatData
	dataPointNames  []string
	dataPoints      map[string]*Stat
	listeners       []chan<- bool
}

func (s *Stat) Reset() {
	s.Lock()
	defer s.Unlock()
	s.data = nil
	s.dataPointNames = []string{}
	s.dataPoints = make(map[string]*Stat)
}

func (s *Stat) Load() (err error) {
	s.Lock()
	defer s.Unlock()
	s.data, err = s.DataLoader.Load(s)
	if err != nil {
		return err
	}

	s.dataPointNames, err = s.DataPointLoader.DataPointNames()
	if err != nil {
		return err
	}

	for _, dpName := range s.dataPointNames {
		dp := Stat{
			Name:            dpName,
			DataLoader:      s.DataPointLoader.NewDataLoader(s.DataLoader, dpName),
			DataPointLoader: s.DataPointLoader.NewDataPointLoader(dpName),
			Depth:           s.Depth + 1,
		}
		if err := dp.Reload(); err != nil {
			return err
		}
		s.dataPoints[dpName] = &dp
	}

	return err
}

func (s *Stat) Reload() error {
	s.Reset()
	return s.Load()
}

func (s *Stat) RefreshData() error {
	s.Lock()
	defer s.Unlock()
	return s.data.Refresh()
}

func (s *Stat) RefreshDataPoints() error {
	for _, dpName := range s.dataPointNames {
		if err := s.dataPoints[dpName].Refresh(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Stat) Refresh() error {
	if err := s.RefreshData(); err != nil {
		return err
	}
	return s.RefreshDataPoints()
}

func (s *Stat) MarshalJSON() ([]byte, error) {
	s.Lock()
	defer s.Unlock()
	dataJSON, err := json.Marshal(s.data)
	if err != nil {
		return nil, fmt.Errorf("json marshalling stat %s data: ", err)
	}
	dataPointsJSON, err := json.Marshal(s.dataPoints)
	if err != nil {
		return nil, fmt.Errorf("json marshalling stat %s dataPoints: ", err)
	}

	return []byte(fmt.Sprintf(`{"name":"%s","data":%s,"dataPoints":%s}`, s.Name, dataJSON, dataPointsJSON)), nil
}

func (s *Stat) String() string {
	s.Lock()
	defer s.Unlock()
	statString := fmt.Sprintf("%s %s %s", s.Name, s.data, s.dataPointNames)
	for _, dp := range s.dataPoints {
		statString += fmt.Sprintf("\n%*s%s", dp.Depth*2, "", dp)
	}
	return statString
}

func (s *Stat) ListenForUpdates(min time.Duration, max time.Duration, errors chan<- error) error {
	if err := s.Reload(); err != nil {
		return err
	}

	updated := make(chan bool)
	s.UpdateListener.Subscribe(updated)

	go func() {
		var (
			ok       bool
			minTimer <-chan time.Time
			maxTimer <-chan time.Time
		)
		for {
			select {
			case _, ok = <-updated:
				if !ok {
					return
				}
				minTimer = time.After(min)
				if maxTimer == nil {
					maxTimer = time.After(max)
				}
			case <-minTimer:
				minTimer, maxTimer = nil, nil
				if err := s.Refresh(); err != nil {
					errors <- fmt.Errorf("%v Stat.refresh(): %v", s.Name, err)
					return
				}
				s.NotifyListeners()
			case <-maxTimer:
				minTimer, maxTimer = nil, nil
				if err := s.Refresh(); err != nil {
					errors <- fmt.Errorf("%v Stat.refresh(): %v", s.Name, err)
					return
				}
				s.NotifyListeners()
			}
		}
	}()

	return nil
}

func (s *Stat) RegisterListener(listener chan<- bool) {
	s.Lock()
	defer s.Unlock()
	s.listeners = append(s.listeners, listener)
}

func (s *Stat) NotifyListeners() {
	for _, listener := range s.listeners {
		listener <- true
	}
}
