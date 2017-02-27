package arithmospora

import (
	"reflect"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
)

type RedisConfig struct {
	Server      string
	DB          int
	MaxIdle     int
	IdleTimeout int
}

var redisConfig = RedisConfig{Server: ":6379", DB: 1, MaxIdle: 3, IdleTimeout: 240}
var redisPool *redis.Pool

func SetRedisConfig(config RedisConfig) {
	if config != (RedisConfig{}) {
		redisConfig = config
	}
}

func RedisPool() *redis.Pool {
	if redisPool == nil {
		redisPool = &redis.Pool{
			MaxIdle:     redisConfig.MaxIdle,
			IdleTimeout: time.Duration(redisConfig.IdleTimeout) * time.Second,
			Dial: func() (redis.Conn, error) {
				c, err := redis.Dial("tcp", redisConfig.Server)
				if err != nil {
					return nil, err
				}
				if _, err := c.Do("SELECT", redisConfig.DB); err != nil {
					c.Close()
					return nil, err
				}
				return c, nil
			},
		}
	}
	return redisPool
}

func makeKey(elements ...string) string {
	return strings.Join(elements, ":")
}

func RedisMakeKey(elements ...string) string {
	return makeKey(elements...)
}

type RedisKeyMaker struct {
	RedisPrefix string
}

func (rkb *RedisKeyMaker) MakeKey(suffices ...string) string {
	return makeKey(append([]string{rkb.RedisPrefix}, suffices...)...)
}

func (rkb *RedisKeyMaker) SetRedisPrefix(newPrefix string) {
	rkb.RedisPrefix = newPrefix
}

func (rkb *RedisKeyMaker) String() string {
	return rkb.RedisPrefix
}

type RedisDataLoader interface {
	StatDataLoader
	SetRedisPrefix(string)
}

type RedisDataPointLoader struct {
	RedisKeyMaker
}

func (rdpl *RedisDataPointLoader) DataPointNames() ([]string, error) {
	conn := RedisPool().Get()
	defer conn.Close()

	return redis.Strings(conn.Do("SMEMBERS", rdpl.MakeKey("datapoints")))
}

func (rdpl *RedisDataPointLoader) NewDataLoader(sdl StatDataLoader, dpName string) StatDataLoader {
	val := reflect.ValueOf(sdl)
	if val.Kind() == reflect.Ptr {
		val = reflect.Indirect(val)
	}
	rdl := reflect.New(val.Type()).Interface().(RedisDataLoader)
	rdl.SetRedisPrefix(rdpl.MakeKey("datapoints", dpName))
	return rdl
}

func (rdpl *RedisDataPointLoader) NewDataPointLoader(dpName string) StatDataPointLoader {
	return &RedisDataPointLoader{RedisKeyMaker{RedisPrefix: rdpl.MakeKey("datapoints", dpName)}}
}

type RedisUpdateListener struct {
	RedisKeyMaker
}

func (rul *RedisUpdateListener) Subscribe(updated chan<- bool) {
	go func() {
		for {
			c := RedisPool().Get()
			psc := redis.PubSubConn{Conn: c}
			defer psc.Close()

			psc.Subscribe(rul.MakeKey("updates"))

			for c.Err() == nil {
				switch psc.Receive().(type) {
				case redis.Message:
					updated <- true
				}
			}
		}
	}()
}
