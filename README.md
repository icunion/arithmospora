Arithmospora
============

Arithmospora spreads numbers.

Arithmospora reads sources of data and broadcasts it to connected clients
via websockets.  It listens for updates to that data so that updates can be
broadcast to clients in near realtime.

Its primary use case is for broadcasting live statistics for dynamic events
such as online elections.

## How it works

Sources consists of collections of data called *stats*, which are usually
grouped together by the type of data held by each stat.  Stat data can
potentially be sourced from any kind of database, however data loaders have
only been implmented for [Redis](https://redis.io) so far.  Stats can also
listen for changes to their data, which is currently implmented by way of
Redis [PUB/SUB](https://redis.io/topics/pubsub).

### Client handling and messages

Each source is exposed as a websocket endpoint, e.g. a source named
*election2017* can be accessed via
`wss://server.hostname:port/election2017`.  Clients connecting to an
endpoint receive JSON message in the following format:

```
{
  "event": "event:name",
  "payload": {...}
}
```

On connection, a client is sent an `available` event message, the payload of
which being the groups and names of all the stats available from this
source.  This allows clients to set up listeners for the stats it is
interested in following.  After a short delay (100ms by default) the client
is then sent data message for all the source's stats.  Further data events
are then sent as and when each stat updates.

All messages from the client are currently ignored, however in future
support may be added for clients to send message in order to only subscribe
to the stats they are interested in, rather than simply receiving all stats.

Stat data messages have event names of the form
`stats:<statGroup>:<statName>`, e.g.  `stats:other:totalvotes`.  The payload
takes the general form:

```
{
  "name": "<statName>",
  "data": {...}
  "dataPoints":{
    "<dp1>": {...},
    "<dp2>": {...},
    ...
    "<dpN>": {...}
  }
}
```

Data points are structured in the same way as the parent stat, allowing a
stat to include a number of related sets of data.  For example, a student
election turnout can be broken down by the study type of voters
(Undergraduate, Postgraduate Taught, and Postgraduate Research), which would
look something like this:

```
{
  "name": "studytypes",
  "data": {...}
  "dataPoints": {
    "PG": {
      "name": "PG",
      "data": {...},
      "dataPoints": {
        "T": {
          "name": "T"
          "data": {...}
          "dataPoints": {}
        },
        "R":
          "name": "R"
          "data": {...}
          "dataPoints": {}
        }
      }
    },
    "UG": {
      ...
    }
  }
}
```

### Stat types

There are currently five different type of stats supported by Arithmospora:
*single value*, *generic*, *proportion*, *rolling*, and *timed*.  The system
can be readily extended to add support for further stat types.

#### Single value stats

Single value stats are the simplest: they consist of a single datum, such as
the total number of votes cast in an election.  Their data is encoded as
`{"<statName>": <value>}`.  For example:

```
{
  "name": "totalvotes",
  "data": {
    "totalvotes": 2345
  }
}
```

#### Generic stats

Generic stats consist of free-form key/value pairs, straightforwardly
encoded directly as a javascript object.

#### Proportion stats

Proportion stats have the following fields:

* `current`
* `total`
* `proportion`
* `percentage`

For eaxample:

```
{
  "current": 71,
  "total": 200,
  "proportion": 0.355,
  "percentage": 35.5
}
```

#### Rolling stats

Rolling stats are similar to proportion stats, but provide their data for a
rolling time frame, such as the last five minutes.  As such, their current
value is expected to go down as well as up.  Rolling stats add three extra
fields to provide information about the busiest period: `peak`,
`peakProportion`, and `peakPercentage`.

#### Timed stats

This is the most complicated type currently supported. Data consists of
key/value pairs with each key corresponding to a time bucket of fixed size. 
The collection of buckets provide a time series of data, such as number of
vote cast in successive five minute periods.

## Installation and usage

### Installation

Arithmospora is written in [Go](https://golang.org/). Install to your go
workspace as follows:

```
go install github.com/icunion/arithmospora/...
```

### Configuration

Arithmospora is configured using a [TOML](https://github.com/toml-lang/toml)
configuration file.  A fully annotated sample configuration file is provided
in `sample.conf`

### Usage

There are three commands provided. All three commands take `-c` flag to
provide the path to the configuration file, and provide any further options
by being invoked with `-help`.  The commands are:

* `arithmospora` - this is the main program. When executed it loads the
  sources specified by the configuration, establish a webserver to serve
  websockets, and will continue to run until aborted.
* `aslist` - loads all stats from a given source and prints them to stdout.
  Supports printing in a human readable text representation or JSON output,
  which can optionally be pretty printed.
* `aswatch` - prints stats to stdout when they update; continues to run
  until aborted.

## About

Arithmospora was created by [Imperial College
Union](https://www.imperialcollegeunion.org) for its live election
statistics to replace a previous [Socket.IO](https://socket.io) system,
which worked by polling a data output script and distributing the results to
clients.  The first use of Arithmospora is to be the [Leadership Elections
2017 Live
Statistics](https://www.imperialcollegeunion.org/leadership-elections-2017/stats/dashboard).

Copyright (c) 2017 Imperial College Union

License: [MIT](https://opensource.org/licenses/MIT)
