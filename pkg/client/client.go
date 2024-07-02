package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mailsac/dracula/pkg/client/serverpool"
	"github.com/mailsac/dracula/pkg/client/waitingmessage"
	"github.com/mailsac/dracula/pkg/protocol"
	"github.com/mailsac/dracula/pkg/server/rawmessage"
)

var (
	ErrInitNoServers            = errors.New("missing dracula udp server list on client init!")
	ErrMessageTimedOut          = errors.New("timed out waiting for message response")
	ErrClientAlreadyInit        = errors.New("client already initialized")
	ErrCountReturnBytesTooShort = errors.New("too few bytes returned in count callback")
	ErrNoHealthyUDPServers      = errors.New("no healthy dracula udp servers")
	ErrNoHealthyTCPServers      = errors.New("no healthy dracula tcp servers")
)

type Client struct {
	// conn is this client's incoming udp listen connection
	conn *net.UDPConn
	// udpPool is the list of remote udp dracula server
	udpPool *serverpool.Pool
	// tcpPool is the list of remote tcp dracula servers
	tcpPool    *sync.Pool
	tcpPoolMap *sync.Map

	tcpServerList []net.TCPAddr

	messagesWaiting *waitingmessage.ResponseCache // byte is the expected response command type

	messageIDCounter uint32
	preSharedKey     []byte

	disposed        bool
	timeoutDuration time.Duration
	log             *log.Logger
}

// Config for the client
type Config struct {
	RemoteUDPIPPortList string
	RemoteTCPIPPortList string
	Timeout             time.Duration
	PreSharedKey        string
}

func NewClient(conf Config) *Client {
	var servers []*net.UDPAddr
	if conf.Timeout == 0 {
		conf.Timeout = time.Second
	}
	client := &Client{
		preSharedKey:    []byte(conf.PreSharedKey),
		messagesWaiting: waitingmessage.NewCache(conf.Timeout),
		log:             log.New(os.Stdout, "", 0),
		tcpPoolMap:      &sync.Map{},
		timeoutDuration: conf.Timeout,
	}

	udpParts := strings.Split(strings.Trim(conf.RemoteUDPIPPortList, " "), ",")
	tcpParts := strings.Split(strings.Trim(conf.RemoteTCPIPPortList, " "), ",")

	for _, ipPort := range udpParts {
		p := strings.Split(strings.Trim(ipPort, " "), ":")
		if p[0] == "" {
			continue
		}
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
	client.udpPool = serverpool.NewPool(client, servers)

	// now parse tcp servers - not required
	if tcpParts[0] != "" {
		for _, ipPort := range tcpParts {
			p := strings.Split(strings.Trim(ipPort, " "), ":")
			if p[0] == "" {
				continue
			}
			if len(p) != 2 {
				panic(fmt.Errorf("bad <ip:port> dracula tcp client init %s", ipPort))
			}
			sport, err := strconv.Atoi(p[1])
			if err != nil {
				panic(fmt.Errorf("bad ip:<port> dracula tcp client init %s", ipPort))
			}
			client.tcpServerList = append(client.tcpServerList, net.TCPAddr{
				IP:   net.ParseIP(p[0]),
				Port: sport,
			})
		}
	}

	if len(servers) == 0 && len(client.tcpServerList) == 0 {
		panic(ErrInitNoServers)
	}

	// setup the pool
	client.tcpPool = &sync.Pool{
		New: func() interface{} {
			// Create a new net.TCPConn object for each server.

			if len(client.tcpServerList) < 1 {
				return nil
			}
			const maxTries = 5
			for i := 0; i < maxTries; i++ {
				randServer := client.tcpServerList[rand.Intn(len(client.tcpServerList))]
				conn, err := net.DialTCP("tcp", nil, &randServer)
				if err != nil {
					client.log.Println("Connection to tcp dracula failed", randServer.String(), err)
				}
				client.tcpPoolMap.Store(conn, true)
				return conn
			}

			return nil
		},
	}

	client.DebugDisable()
	return client
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

	c.udpPool.Listen()
	c.log.Printf("client created server udpPool %v\n", c.udpPool.ListServers())

	return nil
}

func (c *Client) Close() error {
	var err error
	if c.disposed {
		return nil
	}
	c.disposed = true
	c.messagesWaiting.Dispose()

	if c.udpPool != nil {
		c.udpPool.Dispose()
	}
	if c.conn != nil {
		err = c.conn.Close()
		if err != nil {
			return err
		}
	}

	c.tcpPoolMap.Range(func(key, value interface{}) bool {
		conn := key.(*net.TCPConn)
		if conn != nil {
			conn.Close()
		}
		c.tcpPoolMap.Delete(key)
		return true
	})

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

// KeyMatch asks for the list of keys over TCP which match the pattern
func (c *Client) KeyMatch(namespace, keyPattern string) ([]string, error) {
	messageID := c.makeMessageID()
	var wg sync.WaitGroup
	var output string
	var err error
	cb := func(b []byte, e error) {
		defer wg.Done()

		if e != nil {
			err = e
			return
		}
		output = string(b)
	}
	wg.Add(1)
	// callback has been setup, now make the request
	sendPacket := protocol.NewPacketFromParts(protocol.CmdTCPOnlyKeys, messageID, []byte(namespace), []byte(keyPattern), c.preSharedKey)
	c.sendOrCallbackErr(sendPacket, cb)

	wg.Wait() // wait for callback to be called
	results := strings.Split(output, "\n")
	if results[0] == "" {
		results = []string{}
	}
	return results, err
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
	c._sendUDP(p, specificServer, cb)

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

func (c *Client) ListNamespaces() ([]string, error) {
	var err error
	messageID := c.makeMessageID()
	wg := new(sync.WaitGroup)
	namespaces := ""
	cb := func(b []byte, e error) {
		defer wg.Done()
		if e != nil {
			err = e
			return
		}
		namespaces = string(b)
	}
	wg.Add(1)
	sendPacket := protocol.NewPacketFromParts(protocol.CmdTCPOnlyNamespaces, messageID, []byte{}, []byte{}, c.preSharedKey)
	c.sendOrCallbackErr(sendPacket, cb)
	wg.Wait()
	return strings.Split(namespaces, "\n"), err
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

func (c *Client) _sendUDP(packet *protocol.Packet, remoteServer *net.UDPAddr, cb waitingmessage.Callback) {
	c.log.Println("client sending udp packet:", remoteServer, string(packet.Command), packet.MessageID, packet.NamespaceString(), packet.DataValueString())

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
func (c *Client) _sendTCP(packet *protocol.Packet, cb waitingmessage.Callback) {
	c.log.Println("client sending tcp packet:", string(packet.Command), packet.MessageID, packet.NamespaceString(), packet.DataValueString())

	// Get a connection from the pool.
	key := c.tcpPool.Get()
	conn := key.(*net.TCPConn)
	defer func() {
		if conn == nil {
			c.tcpPoolMap.Delete(key)
		} else {
			c.tcpPool.Put(conn)
		}
	}()
	if conn == nil {
		cb([]byte{}, ErrNoHealthyTCPServers)
		return
	}

	// needs stop
	packet.DataValue = append(packet.DataValue, protocol.StopSymbol...)

	packetBuf, err := packet.Bytes()
	if err != nil {
		// probably bad packet
		cb([]byte{}, err)
		return
	}

	resChan := make(chan *rawmessage.RawMessage)
	defer close(resChan)

	var msgErr error
	// handle timeout
	_ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel() // Release resources if operation completes before timeout
	readOneMessage := func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				c.log.Println("tcp connection timed out", packet.Command, packet.MessageID)
				conn.Close()
				packet.DataValue = []byte(ErrMessageTimedOut.Error())
				errPacket, _ := packet.Bytes()
				resChan <- &rawmessage.RawMessage{
					Message:        errPacket,
					Remote:         nil,
					MaybeTcpClient: conn,
				}
				conn = nil
				return
			default:
				msgErr = rawmessage.ReadOneTcpMessage(c.log, resChan, conn)
				if msgErr == nil {
					return
				}
				packet.DataValue = []byte(msgErr.Error())
				errPacket, otherErr := packet.Bytes()
				if otherErr != nil && err != protocol.ErrBadOutputSize {
					c.log.Println("failed constructing error packet after bad tcp read message", msgErr, otherErr)
					return
				}
				resChan <- &rawmessage.RawMessage{
					Message:        errPacket,
					Remote:         nil,
					MaybeTcpClient: conn,
				}
			}
		}
	}

	// we are now waiting for the response, so send the message
	_, err = conn.Write(packetBuf)
	if err != nil {
		c.log.Println("client tcp write failed", err)
		cb([]byte{}, err)
		conn = nil
		return
	}
	go readOneMessage(_ctx)

	// block waiting for response
	res := <-resChan

	// ok

	// tcp callbacks are direct, they don't go through waitingmessages on a separate
	// port, but don't callback the entire packet
	resPacket, err := protocol.ParsePacket(*protocol.PadRight(&res.Message, protocol.PacketSize))
	if err != nil && err != protocol.ErrInvalidPacketSizeTooLarge {
		c.log.Println("client tcp parse res packet failed", err, "|"+string(res.Message)+"|")
		cb([]byte{}, err)
		return
	}
	cb(bytes.TrimSpace(resPacket.DataValue), nil)
}

func (c *Client) sendOrCallbackErr(packet *protocol.Packet, cb waitingmessage.Callback) {
	if protocol.IsTcpOnlyCmd(packet.Command) {
		c._sendTCP(packet, cb)
		return
	}
	remoteServer := c.udpPool.Choose()
	if remoteServer == nil {
		c.log.Println("No healthy udp servers")
		cb([]byte{}, ErrNoHealthyUDPServers)
		return
	}
	c._sendUDP(packet, remoteServer, cb)
}
