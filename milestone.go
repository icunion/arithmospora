package arithmospora

import (
	"fmt"
	"sync"
	"time"
)

type MilestoneValuer interface {
	MilestoneValue(string) float64
}

type Milestone struct {
	sync.Mutex   `json:"-"`
	Name         string    `json:"name"`
	DataPoints   []string  `json:"dataPointNames"`
	Field        string    `json:"field"`
	Target       float64   `json:"target"`
	Comparator   string    `json:"comparator"`
	Message      string    `json:"message"`
	Achieved     bool      `json:"-"`
	AchievedWhen time.Time `json:"achievedWhen"`
}

func (m *Milestone) NewlyMet(stat *Stat) bool {
	m.Lock()
	defer m.Unlock()

	if m.Achieved {
		return false
	}

	for _, dpName := range m.DataPoints {
		if stat.dataPoints[dpName] == nil {
			return false
		}
		stat = stat.dataPoints[dpName]
	}

	data, ok := stat.data.(MilestoneValuer)
	if !ok {
		return false
	}
	value := data.MilestoneValue(m.Field)

	var result bool
	switch m.Comparator {
	case ">":
		result = (value > m.Target)
	case ">=":
		result = (value >= m.Target)
	case "=":
		result = (value == m.Target)
	case "<=":
		result = (value <= m.Target)
	case "<":
		result = (value < m.Target)
	}

	if result {
		m.Achieved = true
		m.AchievedWhen = time.Now()
	}

	return result
}

func (m *Milestone) String() string {
	m.Lock()
	defer m.Unlock()
	return fmt.Sprintf("%v", *m)
}

type MilestoneCollection struct {
	Name       string
	Stat       *Stat
	Milestones []*Milestone
}

func (mc *MilestoneCollection) Publish(achieved chan<- *Milestone) {
	// Check milestones to see if they have already achieved before the program
	// started
	for _, milestone := range mc.Milestones {
		_ = milestone.NewlyMet(mc.Stat)
	}

	// Listen for updates from stat and publish when milestones are met
	go func() {
		statUpdated := make(chan bool)
		mc.Stat.RegisterListener(statUpdated)
		for {
			_ = <-statUpdated
			for _, milestone := range mc.Milestones {
				if milestone.NewlyMet(mc.Stat) {
					achieved <- milestone
				}
			}
		}
	}()
}
