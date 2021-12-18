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

### Download executable

Pre-built binaries of `dracula-server` and `dracula-cli` are [available in the Releases tab](https://github.com/mailsac/dracula/releases).

### Run from docker

dracula container images are published to:
- https://github.com/mailsac/dracula/pkgs/container/dracula

Example running the server:

```bash
docker run -d --rm -p "3509:3509/udp" --name dracula-server-test "ghcr.io/mailsac/dracula" /app/dracula-server -v
```

The dracula CLI is also included in the docker image. Dracula works from IP addresses, so to use docker you must
first get the dracula server's container IP:

```bash
DOCKER_DRACULA_IP=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' dracula-server-test)
echo ${DOCKER_DRACULA_IP}
```

then it is possible to use the docker `dracula-cli` and make a request to count some records:  
```bash
docker run --rm "ghcr.io/mailsac/dracula" /app/dracula-cli -i ${DOCKER_DRACULA_IP} -put -k testing_key
docker run --rm "ghcr.io/mailsac/dracula" /app/dracula-cli -i ${DOCKER_DRACULA_IP} -count -k testing_key
# 1
```

### Build from source

Alternatively, build it (Golang 1.16+ recommended):

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
	// include just one server or comma-separated pool
    serverIPPortPool = "127.0.0.1:3509,192.168.0.1:3509"
	namespace  = "default"
)

var (
	clientResponseTimeout = time.Second * 6
)

func main() {
	c := client.NewClient(serverIPPortPool, clientResponseTimeout, "very-secure3")
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

## High Availability / Failover

Rudimentary and experimental HA is possible via replication by using the `-p` peers list and `-i` self `IP:host` pair flags such as:
```
dracula-server -p "127.0.0.1:3509,127.0.0.1:3519,127.0.0.1:3529" -i 127.0.0.1:3529
```

where clients can connect to the pool and maintain a list of `-i` servers:
```
dracula-cli -i "127.0.0.1:3509,127.0.0.1:3519,127.0.0.1:3529" [...more flags]
```

All peers in the cluster are listed, as well as the self IP and host in the cluster. These flags tell the dracula server to replicate all PUT messages to peers.

In practice, replication only meets the use case of short-lived, imperfectly consistent metrics.

If you require exact replication across peers, this feature will not be tolerant to network partitioning and will not meet your needs.

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

- Persistence
- Better support for high availability under network partitions 
- Clients in other languages
- Retries
- Pipelining

Please open an Issue to request a feature.

## License

See dependencies listed in go.mod for copyright notices and licenses.

----

Copyright (c) 2021 Forking Software LLC

| Project           | License SPDX Identifier |
|-------------------|-------------------------|
| Dracula CLI       | LGPL-3.0 |
| Dracula Go Client | LGPL-3.0 |
| Dracula Server    | GPL-2.0 |

See the files LICENSE.clients.txt and LICENSE.server.txt at the root of this repo.

Dual licensing is available. Contact Forking Software LLC.
