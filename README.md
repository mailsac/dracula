# In-Memory Expirable Key Counter

This is a fast metrics server, ideal for tracking throttling. Put values to the server, and then count them. Values
expire according to the TTL seconds you set.

This repo provides both the client and server.

## Why

One would think Redis is a natural choice for this type of service. When you need to count short-lived, namespaced
keys, Redis does not exactly meet this use case. There are solutions but significant application level code is
required, or Redis needs to be wrapped in another service.

For example, one solution in Redis is to put everything at the top level like `namespace:key:entry-id` and
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
- AWS graviton / ARM support
- easy deploy

We were able to achieve these goals in a server that uses about 1.2MB of RAM on startup.

## Usage

Pre-build binaries of dracula-server and dracula-cli are available in the Releases tab.

Alternatively, build it (Golang required):

```
make build-server
```

### First Run and Test

Optionally set a pre-shared signing secret via environment variable:

```
export DRACULA_SECRET=very-secure3;
```

then run the server with default settings and verbose logging:

```
./dracula-server -v
```

Then use the cli for testing. Put keys to the server:

```
make build-cli

./dracula-cli -put -k asdf
./dracula-cli -put -k asdf

./dracula-cli -count -k asdf
# > 2
```

Keys can be namespaced with the `-n` flag.


Dracula server can be embedded in your Go application:

```go
package main

import (
	"github.com/mailsac/dracula/server"
)

func main() {
	var expireAfterSeconds int64 = 60
	preSharedSecret := "supersecret"
	s := server.NewServer(expireAfterSeconds, preSharedSecret)
	err := s.Listen(3509)
	if err != nil {
		panic(err)
	}
}

```

then use the Go client to put values and count them:

```go
package main

import (
	"fmt"
	"github.com/mailsac/dracula/client"
	"time"
)

const (
	serverPort = 3509
	namespace  = "default"
)

var (
	clientResponseTimeout = time.Second * 6
)

func main() {
	c := client.NewClient("127.0.0.1", serverPort, clientResponseTimeout, "very-secure3")
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
be a throttling server, so missing a few messages wasn't a big deal. Also, UDP can cause TCP traffic on the same box to
be slower under heavy load. This was ideal for our use-case, but may not be for yours.

A message is limited to 1500 bytes. See `protocol/` for exactly how messages are parsed.

The namespace can be 64 bytes and the data value can be 1419 bytes.

The maximum entries in a key is the highest value of uint32.

Authentication is just strong enough to make sure you aren't sending messages to the wrong server. It is assumed dracula
is running in a trusted environment.

## Roadmap

- High Availability
- Persistence
- Clients in other languages
- Retries
- Pipelining

Please open an Issue to request a feature.

## License

See dependencies listed in go.mod for copyright notices and licenses.

----

Copyright (c) 2021 Forking Software LLC

AGPLv3 License

See the LICENSE file at the root of this repo.

Dual licensing is available. Contact Forking Software LLC.
