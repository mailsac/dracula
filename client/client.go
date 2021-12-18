package client

import (
	"errors"
	"fmt"
	"github.com/mailsac/dracula/client/serverpool"
	"github.com/mailsac/dracula/client/waitingmessage"
	"github.com/mailsac/dracula/protocol"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrMessageTimedOut          = errors.New("timed out waiting for message response")
	ErrClientAlreadyInit        = errors.New("client already initialized")
	ErrCountReturnBytesTooShort = errors.New("too few bytes returned in count callback")
	ErrNoHealthyServers         = errors.New("no healthy dracula servers")
)

type Client struct {
	// conn is this clients incoming listen connection
	conn *net.UDPConn
	// pool is the list of servers it will communciate with
	pool *serverpool.Pool
	//remoteServer    *net.UDPAddr
	messagesWaiting *waitingmessage.ResponseCache // byte is the expected response command type

	messageIDCounter uint32
	preSharedKey     []byte

	disposed bool
	log      *log.Logger
}

func NewClient(remoteServerIPPortList string, timeout time.Duration, preSharedKey string) *Client {
	var servers []*net.UDPAddr
	parts := strings.Split(remoteServerIPPortList, ",")
	if len(parts) < 1 {
		panic("missing dracula server list on client init!")
	}
	for _, ipPort := range parts {
		p := strings.Split(ipPort, ":")
		if len(p) != 2 {
			panic(fmt.Errorf("bad <ip:port> dracula client init %s", ipPort))
		}
		sport, err := strconv.Atoi(p[1])
		if err != nil {
			panic(fmt.Errorf("bad ip:<port> dracula client init %s", ipPort))
		}
		servers = append(servers, &net.UDPAddr{
			IP:   net.ParseIP(p[0]),
			Port: sport,
		})
	}
	c := &Client{
		preSharedKey:    []byte(preSharedKey),
		messagesWaiting: waitingmessage.NewCache(timeout),
		log:             log.New(os.Stdout, "", 0),
	}
	c.pool = serverpool.NewPool(c, servers)

	c.DebugDisable()
	return c
}

func (c *Client) GetConn() *net.UDPConn {
	return c.conn
}

func (c *Client) DebugEnable(prefix string) {
	c.log.SetOutput(os.Stdout)
	c.log.SetPrefix(prefix + " ")
}

func (c *Client) DebugDisable() {
	c.log.SetOutput(ioutil.Discard)
}

func (c *Client) PendingRequests() int {
	return c.messagesWaiting.Len()
}

func (c *Client) Listen(localUDPPort int) error {
	if c.conn != nil {
		return ErrClientAlreadyInit
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: localUDPPort,
		IP:   net.ParseIP("0.0.0.0"),
	})
	if err != nil {
		return err
	}
	//defer conn.Close()
	c.conn = conn
	c.log.Printf("client listening %s\n", conn.LocalAddr().String())

	go c.handleResponsesForever()
	go c.handleTimeouts()

	c.pool.Listen()
	c.log.Printf("client created server pool %v\n", c.pool.ListServers())

	return nil
}

func (c *Client) Close() error {
	var err error
	if c.disposed {
		return nil
	}
	c.disposed = true
	c.messagesWaiting.Dispose()

	if c.pool != nil {
		c.pool.Dispose()
	}
	if c.conn != nil {
		err = c.conn.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) handleTimeouts() {
	for timedOutCallback := range c.messagesWaiting.TimedOutMessages {
		timedOutCallback([]byte{}, ErrMessageTimedOut)
		if c.disposed {
			break
		}
	}
}

func (c *Client) handleResponsesForever() {
	for {
		if c.disposed {
			break
		}
		message := make([]byte, protocol.PacketSize)
		_, remote, err := c.conn.ReadFromUDP(message[:])
		if err != nil {
			c.log.Println("client read error:", err)
			continue
		}
		packet, err := protocol.ParsePacket(message)
		if err != nil {
			if packet != nil && packet.MessageID > 0 {
				c.log.Println("client parse packet error but has message id:", packet.MessageID, remote, err, message)
			} else {
				c.log.Println("client received invalid packet from", remote, err, message)
				continue
			}
		}

		c.log.Println("client received packet:", remote, string(packet.Command), packet.MessageID, packet.NamespaceString(), packet.DataValueString())

		cb, err := c.messagesWaiting.Pull(packet.MessageID)
		if err != nil {
			c.log.Println("client message not expected:", packet.Command, packet.MessageID, packet.NamespaceString(), err)
			continue
		}

		if !protocol.IsResponseCmd(packet.Command) {
			c.log.Println("client message not response command:", packet.Command, packet.MessageID, packet.NamespaceString())
			continue
		}

		// handle packet error by constructing error from data value
		if packet.Command == protocol.ResError {
			cb([]byte{}, errors.New(packet.DataValueString()))
			continue
		}

		if packet.Command == protocol.CmdCount || packet.Command == protocol.CmdPut || packet.Command == protocol.CmdCountNamespace || packet.Command == protocol.CmdCountServer {
			cb(packet.DataValue, nil)
			continue
		}

		c.log.Println("client unhandled valid response!", packet.Command, packet.MessageID, packet.NamespaceString())
	}
}

func (c *Client) makeMessageID() []byte {
	id := atomic.AddUint32(&c.messageIDCounter, 1)
	return protocol.Uint32ToBytes(id)
}

// Count asks for the number of unexpired entries in namespace at entryKey. The maximum supported
// number of entries is max of type uint32.
func (c *Client) Count(namespace, entryKey string) (int, error) {
	messageID := c.makeMessageID()
	var wg sync.WaitGroup
	var output uint32
	var err error
	cb := func(b []byte, e error) {
		if e != nil {
			err = e
		} else if len(b) < 4 {
			c.log.Println("client received too few bytes:", b)
			err = ErrCountReturnBytesTooShort
		} else {
			output = protocol.Uint32FromBytes(b[0:4])
		}
		wg.Done()
	}
	wg.Add(1)
	// callback has been setup, now make the request
	p := protocol.NewPacketFromParts(protocol.CmdCount, messageID, []byte(namespace), []byte(entryKey), c.preSharedKey)
	c.sendOrCallbackErr(p, cb)

	wg.Wait() // wait for callback to be called
	return int(output), err
}

// Healthcheck implements serverpool.Checker
func (c *Client) Healthcheck(specificServer *net.UDPAddr) error {
	messageID := c.makeMessageID()
	var wg sync.WaitGroup
	var err error
	cb := func(b []byte, e error) {
		if e != nil {
			err = e
		}
		wg.Done()
	}
	wg.Add(1)
	// callback has been setup, now make the request
	p := protocol.NewPacketFromParts(protocol.CmdCount, messageID, []byte("server_healthcheck_"+specificServer.String()), []byte("check"), c.preSharedKey)
	c._send(p, specificServer, cb)

	wg.Wait() // wait for callback to be called
	return err
}

// CountNamespace (expensive) returns the number of key entries across all keys in a namespace.
func (c *Client) CountNamespace(namespace string) (int, error) {
	messageID := c.makeMessageID()
	var wg sync.WaitGroup
	var output uint32
	var err error
	cb := func(b []byte, e error) {
		if e != nil {
			err = e
		} else if len(b) < 4 {
			c.log.Println("client received too few bytes:", b)
			err = ErrCountReturnBytesTooShort
		} else {
			output = protocol.Uint32FromBytes(b[0:4])
		}
		wg.Done()
	}
	wg.Add(1)
	// callback has been setup, now make the request
	p := protocol.NewPacketFromParts(protocol.CmdCountNamespace, messageID, []byte(namespace), []byte{}, c.preSharedKey)
	c.sendOrCallbackErr(p, cb)

	wg.Wait() // wait for callback to be called
	return int(output), err
}

// CountServer (very expensive) returns the number of key entries across all keys in all namespaces.
func (c *Client) CountServer() (int, error) {
	messageID := c.makeMessageID()
	var wg sync.WaitGroup
	var output uint32
	var err error
	cb := func(b []byte, e error) {
		if e != nil {
			err = e
		} else if len(b) < 4 {
			c.log.Println("client received too few bytes:", b)
			err = ErrCountReturnBytesTooShort
		} else {
			output = protocol.Uint32FromBytes(b[0:4])
		}
		wg.Done()
	}
	wg.Add(1)
	// callback has been setup, now make the request
	p := protocol.NewPacketFromParts(protocol.CmdCountServer, messageID, []byte{}, []byte{}, c.preSharedKey)
	c.sendOrCallbackErr(p, cb)

	wg.Wait() // wait for callback to be called
	return int(output), err
}

func (c *Client) Put(namespace, value string) error {
	messageID := c.makeMessageID()
	var wg sync.WaitGroup
	var err error
	cb := func(b []byte, e error) {
		err = e
		c.log.Println("client put error", e)
		wg.Done()
	}
	wg.Add(1)
	// callback has been setup, now make the request
	p := protocol.NewPacketFromParts(protocol.CmdPut, messageID, []byte(namespace), []byte(value), c.preSharedKey)
	c.sendOrCallbackErr(p, cb)

	wg.Wait() // wait for callback to be called
	return err
}

func (c *Client) _send(packet *protocol.Packet, remoteServer *net.UDPAddr, cb waitingmessage.Callback) {
	c.log.Println("client sending packet:", remoteServer, string(packet.Command), packet.MessageID, packet.NamespaceString(), packet.DataValueString())

	b, err := packet.Bytes()
	if err != nil {
		// probably bad packet
		cb([]byte{}, err)
		return
	}

	err = c.messagesWaiting.Add(packet.MessageID, cb)
	if err != nil {
		c.log.Println("client failed adding waiting message!", packet.MessageID)
		cb([]byte{}, err)
		return
	}

	_, err = c.conn.WriteToUDP(b, remoteServer)
	if err != nil {
		// immediate failure, handle here
		reCall, pullErr := c.messagesWaiting.Pull(packet.MessageID)
		if pullErr != nil {
			c.log.Println("client failed callback could not be called!", remoteServer, string(packet.Command), packet.MessageID, packet.NamespaceString(), packet.DataValueString())
			reCall = cb
		}
		reCall([]byte{}, err)
		return
	}

	// ok
}

func (c *Client) sendOrCallbackErr(packet *protocol.Packet, cb waitingmessage.Callback) {
	remoteServer := c.pool.Choose()
	if remoteServer == nil {
		cb([]byte{}, ErrNoHealthyServers)
		return
	}
	c._send(packet, remoteServer, cb)
}
