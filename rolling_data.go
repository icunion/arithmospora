package arithmospora

import (
	"fmt"

	"github.com/garyburd/redigo/redis"
)

type RollingDataLoader interface {
	StatDataLoader
	FetchData() ([]int, error)
}

type RollingData struct {
	ProportionData
	Peak       int
	dataLoader RollingDataLoader
}

func (rd *RollingData) PeakProportion() float64 {
	if rd.Total != 0 {
		return float64(rd.Peak) / float64(rd.Total)
	} else {
		return 0
	}
}

func (rd *RollingData) PeakPercentage() float64 {
	return rd.PeakProportion() * 100
}

func (rd *RollingData) Refresh() error {
	data, err := rd.dataLoader.FetchData()
	if err != nil {
		return err
	}

	rd.Current = data[0]
	rd.Total = data[1]
	rd.Peak = data[2]
	return nil
}

func (rd *RollingData) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"current":%v,"total":%v,"peak":%v,"proportion":%v,"percentage":%v,"peakProportion":%v,"peakPercentage":%v}`,
		rd.Current,
		rd.Total,
		rd.Peak,
		rd.Proportion(),
		rd.Percentage(),
		rd.PeakProportion(),
		rd.PeakPercentage())), nil
}

func (rd *RollingData) String() string {
	return fmt.Sprintf("current: %v, total: %v, proportion: %v, percentage: %v, peak: %v, peakProportion: %v (%s)",
		rd.Current,
		rd.Total,
		rd.Proportion(),
		rd.Percentage(),
		rd.Peak,
		rd.PeakProportion(),
		rd.dataLoader)
}

type RollingDataLoaderRedis struct {
	RedisKeyMaker
}

func (rdl *RollingDataLoaderRedis) FetchData() ([]int, error) {
	conn := RedisPool().Get()
	defer conn.Close()

	return redis.Ints(conn.Do("HMGET", rdl.MakeKey("data"), "current", "total", "peak"))
}

func (rdl *RollingDataLoaderRedis) Load(*Stat) (StatData, error) {
	data, err := rdl.FetchData()
	if err != nil {
		return nil, err
	}

	return &RollingData{ProportionData: ProportionData{Current: data[0], Total: data[1]}, Peak: data[2], dataLoader: rdl}, nil
}

func (rdl *RollingDataLoaderRedis) String() string {
	return rdl.RedisKeyMaker.String()
}
