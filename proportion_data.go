package arithmospora

import (
	"fmt"

	"github.com/garyburd/redigo/redis"
)

type ProportionDataLoader interface {
	StatDataLoader
	FetchData() ([]int, error)
}

type ProportionData struct {
	Current    int
	Total      int
	dataLoader ProportionDataLoader
}

func (pd *ProportionData) Proportion() float64 {
	if pd.Total != 0 {
		return float64(pd.Current) / float64(pd.Total)
	} else {
		return 0
	}
}

func (pd *ProportionData) Percentage() float64 {
	return pd.Proportion() * 100
}

func (pd *ProportionData) Refresh() error {
	data, err := pd.dataLoader.FetchData()
	if err != nil {
		return err
	}

	pd.Current = data[0]
	pd.Total = data[1]
	return nil
}

func (pd *ProportionData) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"current":%v,"total":%v,"proportion":%v,"percentage":%v}`, pd.Current, pd.Total, pd.Proportion(), pd.Percentage())), nil
}

func (pd *ProportionData) String() string {
	return fmt.Sprintf("current: %v, total: %v, proportion: %v, percentage: %v (%s)", pd.Current, pd.Total, pd.Proportion(), pd.Percentage(), pd.dataLoader)
}

func (pd *ProportionData) MilestoneValue(field string) float64 {
	switch field {
	case "current":
		return float64(pd.Current)
	case "proportion":
		return pd.Proportion()
	case "percentage":
		return pd.Percentage()
	}
	return 0.0
}

type ProportionDataLoaderRedis struct {
	RedisKeyMaker
}

func (pdl *ProportionDataLoaderRedis) FetchData() ([]int, error) {
	conn := RedisPool().Get()
	defer conn.Close()

	return redis.Ints(conn.Do("HMGET", pdl.MakeKey("data"), "current", "total"))
}

func (pdl *ProportionDataLoaderRedis) Load(*Stat) (StatData, error) {
	data, err := pdl.FetchData()
	if err != nil {
		return nil, err
	}

	return &ProportionData{Current: data[0], Total: data[1], dataLoader: pdl}, nil
}

func (pdl *ProportionDataLoaderRedis) String() string {
	return pdl.RedisKeyMaker.String()
}
