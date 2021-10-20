# In-Memory Expirable Key Counter

This is a fast metrics server, ideal for tracking throttling. Put values to the server, and then count them. Values
expire according to the TTL seconds you set.

This repo provides both the client and server.

## Why

One would think Redis is a natural choice for this type of service. When you need to just count short-lived, namespaced
keys, Redis cannot does not exactly meet this use case. There are solutions but significant application level code is
required, or Redis needs to be wrapped in another service.

One solution in Redis is to put everything at the top level like `namespace:key:entry-id` and
run `SCAN 0 MATCH namespace:key:*`. But it *returns all the keys*, which you can then count. It also requires that you
implement some sort of unique ID generation. Redis does support `KEEPTTL` during `SET` to each key.

Another approach in Redis is to store entries in a sorted set or other data structure, at a key like `namespace:key` and
omit or expire entries in application logic. We also considered adding a service in front of Redis to handle that, but
that seemed about as much overhead as just writing exactly what we needed into this dracula project.

#### Design Requirements

At the outset, we wanted to meet the following needs.

- every key entry has the same expiry time
- every key is in a namespace
- support write-heavy operations
- support tens of thousands of simultaneous reads and writes on low-end hardware

We were able to achieve a service that uses about 1.2MB of RAM on startup.

## Usage

*TODO: build binaries and put in github release*

To use this server, build it (Golang required):

```
make build-server

./dracula -v
```

Then use the cli for testing:

```
make build-cli

./dracula-cli -put -k asdf
./dracula-cli -put -k asdf

./dracula-cli -count -k asdf
# > 2
```

or include it in your Go application:

```go
expireAfterSeconds := 60
s := server.NewServer(expireAfterSeconds)
err := s.Listen(3509)
```

then use the client to put values and count them:

```go
package main

import (
	"time"
	"fmt"
	"github.com/mailsac/dracula/client"
)

const (
	serverPort = 3509
	namespace  = "default"
)
var (
	clientResponseTimeout = time.Second*6
)

func main() {
	c := client.NewClient("127.0.0.1", serverPort, clientResponseTimeout)
	c.Listen(9001)

	// seed some entries
	c.Put(namespace, "192.168.0.50")
	c.Put(namespace, "192.168.0.50")
	c.Put(namespace, "mailsac.com")
	c.Put(namespace, "hello.msdc.co")

	total, _ := c.Count(namespace, "192.168.0.50")
	fmt.Println("192.168.0.50", total) // 2

	// wait a while
	time.Sleep(time.Second * 60)

	// now entries will be expired
	total, _ = c.Count(namespace, "mailsac.com")
	fmt.Println("mailsac.com", total) // 0
}

```

Entries are grouped in a `namespace`.

See `server/server_test.go` for examples.

## Limitations

Messages are sent over UDP and not reliable. The trade-off desired is speed. This project was initially implemented to
be a throttling server, so missing a few messages wasn't a big deal.

A message is limited to 1500 bytes. See `protocol/` for exactly how messages are parsed.

The namespace can be 64 bytes and the data value can be 1428 bytes.

The maximum entries in a key is the highest value of uint32.

## License

See dependencies listed in go.mod for copyright notices and licenses.

----

Copyright (c) 2021 Forking Software LLC

MIT License

See the LICENSE file at the root of this repo.
