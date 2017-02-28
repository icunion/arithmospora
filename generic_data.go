package arithmospora

import (
	"encoding/json"
	"fmt"

	"github.com/garyburd/redigo/redis"
)

type GenericDataLoader interface {
	StatDataLoader
	FetchData() (map[string]int, error)
}

type GenericData struct {
	Data       map[string]int
	dataLoader GenericDataLoader
}

func (gd *GenericData) MarshalJSON() ([]byte, error) {
	return json.Marshal(gd.Data)
}

func (gd *GenericData) String() string {
	return fmt.Sprintf("%v (%s)", gd.Data, gd.dataLoader)
}

func (gd *GenericData) Refresh() error {
	data, err := gd.dataLoader.FetchData()
	if err != nil {
		return err
	}

	gd.Data = data
	return nil
}

func (gd *GenericData) MilestoneValue(field string) float64 {
	return float64(gd.Data[field])
}

type GenericDataLoaderRedis struct {
	RedisKeyMaker
}

func (gdl *GenericDataLoaderRedis) FetchData() (map[string]int, error) {
	conn := RedisPool().Get()
	defer conn.Close()

	return redis.IntMap(conn.Do("HGETALL", gdl.MakeKey("data")))
}

func (gdl *GenericDataLoaderRedis) Load(*Stat) (StatData, error) {
	data, err := gdl.FetchData()
	if err != nil {
		return nil, fmt.Errorf("%s:data %v", gdl, err)
	}

	return &GenericData{Data: data, dataLoader: gdl}, nil
}
