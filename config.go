package arithmospora

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/naoina/toml"
)

type tomlConfig struct {
	Redis     RedisConfig
	Https     HttpsConfig
	Websocket WebsocketConfig
	Debounce  DebounceConfig
	Sources   []SourceConfig
}

type HttpsConfig struct {
	Address string
	Cert    string
	Key     string
}

type DebounceConfig struct {
	MinTimeMs int
	MaxTimeMs int
}

type SourceConfig struct {
	Name             string
	RedisPrefix      string
	StartTime        time.Time
	EndTime          time.Time
	IsLive           bool
	TimedStatPeriods []Period
	Stats            StatGroupConfig
	Milestones       []MilestoneConfig
}

type StatGroupConfig struct {
	Proportion []StatConfig
	Rolling    []StatConfig
	Timed      []StatConfig
	Other      []StatConfig
}

type StatConfig struct {
	Name       string
	Period     string
	DataType   string
	LoaderType string
}

type MilestoneConfig struct {
	Name       string
	Group      string
	Stat       string
	Milestones []*Milestone
}

var Config tomlConfig

func ParseConfig(configFile string) error {
	if configFile == "" {
		return fmt.Errorf("config file not specified")
	}
	buf, err := ioutil.ReadFile(configFile)
	if err != nil {
		return err
	}
	if err := toml.Unmarshal(buf, &Config); err != nil {
		return err
	}
	return nil
}

func MakeSourcesFromConfig(config tomlConfig) (sources []*Source) {
	for _, sourceConfig := range config.Sources {
		source := Source{Name: sourceConfig.Name, IsLive: sourceConfig.IsLive}
		source.Available = make(map[string][]string)
		source.Stats = make(map[string]map[string]*Stat)

		// Proportion stats
		for _, statConfig := range sourceConfig.Stats.Proportion {
			if source.Stats["proportion"] == nil {
				source.Stats["proportion"] = make(map[string]*Stat)
			}
			if statConfig.DataType == "" {
				statConfig.DataType = "proportion"
			}
			stat := MakeStatFromConfig(sourceConfig, statConfig)
			source.Available["proportion"] = append(source.Available["proportion"], stat.Name)
			source.Stats["proportion"][stat.Name] = stat
		}

		// Rolling stats
		for _, statConfig := range sourceConfig.Stats.Rolling {
			if source.Stats["rolling"] == nil {
				source.Stats["rolling"] = make(map[string]*Stat)
			}
			if statConfig.DataType == "" {
				statConfig.DataType = "rolling"
			}
			stat := MakeStatFromConfig(sourceConfig, statConfig)
			source.Available["rolling"] = append(source.Available["rolling"], statConfig.Period+":"+stat.Name)
			source.Stats["rolling"][statConfig.Period+":"+stat.Name] = stat
		}

		// Timed stats
		for _, statConfig := range sourceConfig.Stats.Timed {
			if source.Stats["timed"] == nil {
				source.Stats["timed"] = make(map[string]*Stat)
			}
			if statConfig.DataType == "" {
				statConfig.DataType = "timed"
			}
			stat := MakeStatFromConfig(sourceConfig, statConfig)
			source.Available["timed"] = append(source.Available["timed"], stat.Name)
			source.Stats["timed"][stat.Name] = stat
		}

		// Other stats
		for _, statConfig := range sourceConfig.Stats.Other {
			var statKey string
			if source.Stats["other"] == nil {
				source.Stats["other"] = make(map[string]*Stat)
			}
			stat := MakeStatFromConfig(sourceConfig, statConfig)
			if statConfig.Period != "" {
				statKey = statConfig.Period + ":" + stat.Name
			} else {
				statKey = stat.Name
			}
			source.Available["other"] = append(source.Available["other"], statKey)
			source.Stats["other"][statKey] = stat
		}

		// Milestones
		for _, milestoneConfig := range sourceConfig.Milestones {
			// Skip milestone if corresponding stat not found
			if source.Stats[milestoneConfig.Group] == nil || source.Stats[milestoneConfig.Group][milestoneConfig.Stat] == nil {
				continue
			}

			milestoneCollection := &MilestoneCollection{
				Name:       milestoneConfig.Name,
				Stat:       source.Stats[milestoneConfig.Group][milestoneConfig.Stat],
				Milestones: milestoneConfig.Milestones,
			}
			source.Milestones = append(source.Milestones, milestoneCollection)
		}

		sources = append(sources, &source)
	}
	return sources
}

func MakeStatFromConfig(sourceConfig SourceConfig, statConfig StatConfig) *Stat {
	var (
		dataLoader      StatDataLoader
		dataPointLoader StatDataPointLoader
		updateListener  StatUpdateListener
	)

	if statConfig.LoaderType == "redis" {
		var keyMaker RedisKeyMaker
		if statConfig.Period != "" {
			keyMaker = RedisKeyMaker{makeKey(sourceConfig.RedisPrefix, "rolling", statConfig.Period, "stats", statConfig.Name)}
		} else {
			keyMaker = RedisKeyMaker{makeKey(sourceConfig.RedisPrefix, "stats", statConfig.Name)}
		}
		dataPointLoader = &RedisDataPointLoader{keyMaker}
		updateListener = &RedisUpdateListener{keyMaker}

		switch statConfig.DataType {
		case "generic":
			dataLoader = &GenericDataLoaderRedis{keyMaker}
		case "proportion":
			dataLoader = &ProportionDataLoaderRedis{keyMaker}
		case "rolling":
			dataLoader = &RollingDataLoaderRedis{keyMaker}
		case "single_value":
			dataLoader = &SingleValueDataLoaderRedis{keyMaker}
		case "timed":
			dataLoader = &TimedDataLoaderRedis{
				RedisKeyMaker: keyMaker,
				StartTime:     sourceConfig.StartTime,
				EndTime:       sourceConfig.EndTime,
				Periods:       sourceConfig.TimedStatPeriods,
			}
			dataPointLoader = &TimedDataPointLoaderRedis{
				RedisKeyMaker: keyMaker,
				Periods:       sourceConfig.TimedStatPeriods,
			}
		}
	}

	return &Stat{
		Name:            statConfig.Name,
		DataLoader:      dataLoader,
		DataPointLoader: dataPointLoader,
		UpdateListener:  updateListener,
	}
}
