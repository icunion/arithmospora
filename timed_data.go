package arithmospora

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/garyburd/redigo/redis"
)

type TimedDataLoader interface {
	StatDataLoader
	FetchBucket(int64) (int, error)
}

type Period struct {
	Granularity int64
	Cycles      int64
	BucketKeys  []int64
	Buckets     map[int64]int
}

type TimedData struct {
	StartTime  time.Time
	EndTime    time.Time
	Period     *Period
	dataLoader TimedDataLoader
}

func (td *TimedData) MarshalJSON() ([]byte, error) {
	if td.Period.Granularity == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(td.Period.Buckets)
}

func (td *TimedData) String() string {
	return fmt.Sprintf("%v (%s)", td.Period, td.dataLoader)
}

func (td *TimedData) Refresh() error {
	// If granularity = zero then we have no data so nothing to do
	if td.Period.Granularity == 0 {
		return nil
	}

	// Determine current time:, peg at no further than 5 minutes beyond end time
	// TODO: make configurable
	currentTime := time.Now()
	if currentTime.After(td.EndTime.Add(5 * time.Minute)) {
		currentTime = td.EndTime.Add(5 * time.Minute)
	}

	// Fetch current and previous buckets if within time range
	currentBucket := currentTime.Unix() / td.Period.Granularity
	previousBucket := currentBucket - 1
	startBucket := td.Period.BucketKeys[0]

	if currentBucket >= startBucket {
		newCurrent, err := td.dataLoader.FetchBucket(currentBucket)
		if err != nil {
			return fmt.Errorf("%v: %v", currentBucket, err)
		}
		td.Period.Buckets[currentBucket] = newCurrent
	}

	if previousBucket >= startBucket {
		newPrevious, err := td.dataLoader.FetchBucket(previousBucket)
		if err != nil {
			return fmt.Errorf("%v: %v", previousBucket, err)
		}
		td.Period.Buckets[previousBucket] = newPrevious
	}

	// If we're a moving window and have acquired a new bucket, discard the
	// oldest bucket
	if td.Period.Cycles > 0 && int64(len(td.Period.Buckets)) > td.Period.Cycles+1 {
		delete(td.Period.Buckets, td.Period.BucketKeys[0])
		td.Period.BucketKeys = append(td.Period.BucketKeys[1:], currentBucket)
	}

	return nil
}

type TimedDataLoaderRedis struct {
	RedisKeyMaker
	StartTime time.Time
	EndTime   time.Time
	Periods   []Period
}

func (tdl *TimedDataLoaderRedis) FetchBucket(bucket int64) (int, error) {
	conn := RedisPool().Get()
	defer conn.Close()

	data, err := redis.Int(conn.Do("HGET", tdl.MakeKey("data"), fmt.Sprintf("%v", bucket)))
	if err != nil && err != redis.ErrNil {
		return 0, err
	}
	return data, nil
}

func (tdl *TimedDataLoaderRedis) Load(stat *Stat) (StatData, error) {
	conn := RedisPool().Get()
	defer conn.Close()

	timedData := TimedData{
		StartTime:  tdl.StartTime,
		EndTime:    tdl.EndTime,
		Period:     &Period{},
		dataLoader: tdl,
	}

	// Determine current time:, peg at no further than 5 minutes beyond end time
	// TODO: make configurable
	currentTime := time.Now()
	if currentTime.After(tdl.EndTime.Add(5 * time.Minute)) {
		currentTime = tdl.EndTime.Add(5 * time.Minute)
	}

	// Loop through periods and construct if period matches the datapoint being loaded
	for _, period := range tdl.Periods {
		if fmt.Sprintf("%v", period.Granularity) != stat.Name {
			continue
		}

		period.Buckets = make(map[int64]int)

		// Determine start and end buckets
		var (
			startBucket int64
			endBucket   int64
		)
		if period.Cycles < 0 {
			// Whole period: start bucket is start time, end bucket is end time
			// plus one hour (e.g. to allow for people still in voting booth
			// after end of election)
			startBucket = tdl.StartTime.Unix() / period.Granularity
			endBucket = tdl.EndTime.Add(1*time.Hour).Unix() / period.Granularity
		} else {
			// Moving window based on current time
			endBucket = currentTime.Unix() / period.Granularity
			startBucket = endBucket - period.Cycles
		}

		// Create Buckets with zero values and build list of keys
		period.BucketKeys = make([]int64, endBucket-startBucket+1)
		index := 0
		for key := startBucket; key <= endBucket; key++ {
			period.Buckets[key] = 0
			period.BucketKeys[index] = key
			index++
		}

		// Load data from redis
		dataKey := tdl.MakeKey("data")
		if period.Cycles < 0 {
			// Whole period: Load all data with HGETALL
			data, err := redis.IntMap(conn.Do("HGETALL", dataKey))
			if err != nil {
				return nil, err
			}
			for _, key := range period.BucketKeys {
				period.Buckets[key] = data[fmt.Sprintf("%v", key)]
			}
		} else {
			// Moving window: Load all values in range with HMGET
			redisArgs := make([]interface{}, len(period.BucketKeys)+1)
			redisArgs[0] = dataKey
			for index, key := range period.BucketKeys {
				redisArgs[index+1] = fmt.Sprintf("%v", key)
			}
			data, err := redis.Ints(conn.Do("HMGET", redisArgs...))
			if err != nil {
				return nil, err
			}
			for index, key := range period.BucketKeys {
				period.Buckets[key] = data[index]
			}

		}

		// Assign period to stat
		timedData.Period = &period

		// Period found and contructed - No need to progress further through the loop
		break
	}

	// If we don't have a granularity, then we're in the parent stat
	// Set up a ticker to force refresh the stat (and by extension the
	// children) every time a moving window child stat move along a bucket
	if timedData.Period.Granularity == 0 {
		ticker := time.NewTicker(time.Second)
		go func() {
			for {
				currentTime := <-ticker.C
				// Cancel the ticker if we're past the end time + 5 minutes
				if currentTime.After(tdl.EndTime.Add(5 * time.Minute)) {
					ticker.Stop()
					return
				}

				for _, period := range tdl.Periods {
					if period.Cycles > 0 && currentTime.Unix()%period.Granularity == 0 {
						// Publish stat update through Redis - ignore errors
						conn := RedisPool().Get()
						_, _ = conn.Do("PUBLISH", tdl.MakeKey("updates"), 1)
						conn.Close()

						// Break out of the loop, as we only need to publish
						// once irrespective of the number of moving window
						// children entering a new time bucket
						break
					}
				}
			}
		}()
	}

	return &timedData, nil
}

type TimedDataPointLoaderRedis struct {
	RedisKeyMaker
	Periods []Period
}

func (tdplr *TimedDataPointLoaderRedis) DataPointNames() (dpNames []string, err error) {
	for _, period := range tdplr.Periods {
		dpNames = append(dpNames, fmt.Sprintf("%v", period.Granularity))
	}
	return
}

func (tdplr *TimedDataPointLoaderRedis) NewDataLoader(sdl StatDataLoader, dpName string) StatDataLoader {
	tdl, ok := sdl.(*TimedDataLoaderRedis)
	if !ok {
		return nil
	}

	return &TimedDataLoaderRedis{
		RedisKeyMaker: RedisKeyMaker{RedisPrefix: tdplr.MakeKey("datapoints", dpName)},
		StartTime:     tdl.StartTime,
		EndTime:       tdl.EndTime,
		Periods:       tdl.Periods,
	}
}

func (tdplr *TimedDataPointLoaderRedis) NewDataPointLoader(dpName string) StatDataPointLoader {
	return &TimedDataPointLoaderRedis{RedisKeyMaker{RedisPrefix: tdplr.MakeKey("datapoints", dpName)}, nil}
}
