package arithmospora

import (
	"fmt"

	"github.com/garyburd/redigo/redis"
)

type SingleValueDataLoader interface {
	StatDataLoader
	FetchData() (int, error)
}

type SingleValueData struct {
	Name       string
	Data       int
	dataLoader SingleValueDataLoader
}

func (svd *SingleValueData) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("{ \"%s\": %v }", svd.Name, svd.Data)), nil
}

func (svd *SingleValueData) String() string {
	return fmt.Sprintf("%v (%s)", svd.Data, svd.dataLoader)
}

func (svd *SingleValueData) Refresh() error {
	data, err := svd.dataLoader.FetchData()
	if err != nil {
		return err
	}

	svd.Data = data
	return nil
}

type SingleValueDataLoaderRedis struct {
	RedisKeyMaker
}

func (svdl *SingleValueDataLoaderRedis) FetchData() (int, error) {
	conn := RedisPool().Get()
	defer conn.Close()

	data, err := redis.Int(conn.Do("GET", svdl.MakeKey("data")))
	if err != nil && err != redis.ErrNil {
		return 0, err
	}
	return data, nil
}

func (svdl *SingleValueDataLoaderRedis) Load(stat *Stat) (StatData, error) {
	data, err := svdl.FetchData()
	if err != nil {
		return nil, fmt.Errorf("%s:data %v", svdl, err)
	}

	return &SingleValueData{Name: stat.Name, Data: data, dataLoader: svdl}, nil
}
